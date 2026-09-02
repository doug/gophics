package app

import (
	"encoding/json"
	"log"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// shellHandler adapts core to shell.Handler: the per-frame pipeline (posted
// work, ticks, build, layout, record, replay, present) and window wiring.

type shellHandler struct {
	core   *core
	window shell.Window
	// wired is the window whose hooks/capabilities are currently published to
	// the Owner; wireWindow re-wires only when the shell hands us another one.
	wired shell.Window
	// lastGPU is the GPU target the previous frame rendered to (nil when the
	// previous frame took the CPU path). present() skips the GPU replay only
	// for an unchanged scene on the same target — a target it has never
	// rendered to has never presented this scene (see present.go).
	lastGPU gpuCanvasTarget

	// lastA11y is the accessibility tree most recently handed to the platform
	// bridge, kept so an unchanged tree is not republished every frame.
	lastA11y []A11yNode

	// Dev-mode state-preserving hot-restart (set only under `gophics dev` via
	// setupDevState; zero/no-op in a shipped binary). On a restart signal the
	// handler snapshots UI state to devStatePath so the relaunched process can
	// restore it, landing back at the same place. See devstate_desktop.go.
	devStatePath string
	devQuit      atomic.Bool
	devSaved     bool
}

// writeDevSnapshot serializes snap to path via a temp file + rename, so the
// relaunched process never reads a half-written file.
func writeDevSnapshot(path string, snap widget.StateSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// TextInputActive reports whether a widget currently accepts typed text —
// embedded hosts poll it and raise or dismiss the on-screen keyboard as it
// changes.
//
// The test is OnText, not focus. A widget becomes focusable by handling OnText
// *or* OnKey, so a button that responds to Enter and a list that responds to
// the arrow keys both take focus without wanting a keyboard — and something
// focusable is usually mounted from the first frame, because a focusable
// widget mounted while nothing has focus takes it. Reporting focus therefore
// answered "yes" before any field existed and kept answering it, so the host
// saw no transition and never raised the keyboard: a field that could be
// tapped, showed a caret, and could not be typed into.
func (h *shellHandler) TextInputActive() bool {
	t := h.core.Owner.KeyboardTarget
	return t != nil && t.OnText != nil
}

// Accessibility bridge methods (embedded hosts type-assert the handler).

func (h *shellHandler) A11yTree(scale float32) []A11yNode { return h.core.A11yTree(scale) }
func (h *shellHandler) A11yActivate(id int)               { h.core.A11yActivate(id) }
func (h *shellHandler) A11yHitTest(x, y int, scale float32) int {
	return h.core.A11yHitTest(x, y, scale)
}

func (h *shellHandler) Frame(w shell.Window, f shell.Frame, dt float64) {
	h.window = w
	// Dev hot-restart: a restart signal arrived. Snapshot UI state on the UI
	// goroutine (safe here — no frame is mid-flight), hand it to the successor
	// process, and ask the shell to close. Guarded so it runs once.
	if h.devStatePath != "" && h.devQuit.Load() && !h.devSaved {
		h.devSaved = true
		if snap := h.core.Owner.SnapshotState(); len(snap) > 0 {
			if err := writeDevSnapshot(h.devStatePath, snap); err != nil {
				log.Printf("gophics dev: snapshot state: %v", err)
			}
		}
		w.Close()
		return
	}
	h.wireWindow(w)
	if dark := w.DarkMode(); dark != h.core.Owner.DarkMode {
		h.core.Owner.DarkMode = dark
		h.core.Owner.RebuildAll()
	}
	// Frame pipeline: posted work → tick animations → build →
	// layout → record → diff → replay damage → present.
	h.core.drainPosted()
	h.core.TickGestures(dt)
	if h.core.Owner.TickAll(dt) || h.core.longPressPending() {
		w.Invalidate() // animations or a held long-press: keep frames coming
	}
	t0 := time.Now()
	devices0 := wgpu.DeviceStats()
	// Resolve the presentation target up front: a GPU target replays the whole
	// scene, so the damage rect (and its per-text-op measurement) is never
	// computed for it — see RecordSceneGPU.
	tgt := f.Target()
	changed, damage, ok := h.recordFrame(f, tgt)
	// recordFrame builds, and a Build is where a control starts the animation
	// that reacts to its own new state. TickAll above ran before that, so it
	// could not have seen it; without this the animation never gets a second
	// frame on a device that produces no hover events, and it sits frozen at
	// its start value while the rest of the UI shows the new state.
	if h.core.Owner.TickersActive() {
		w.Invalidate()
	}
	if !ok {
		// Layout or paint panicked: drop this frame, keep the previous one on
		// screen, and keep the app alive (mirrors safeBuild's isolation policy
		// for Build panics — widget/element.go).
		h.presentDropped(f, tgt)
		if in := h.core.Owner.Input; in != nil {
			in.NewFrame()
		}
		return
	}
	// Present via the GPU rasterizer or the CPU rasterizer, chosen per frame
	// from the frame's Target (see present.go).
	h.present(f, tgt, changed, damage)
	if changed {
		// Semantics can only have moved if the frame did, so republishing is
		// gated on the same signal the renderer uses.
		h.publishA11y()
	}
	if in := h.core.Owner.Input; in != nil {
		in.NewFrame() // clear per-frame key/pointer edges after the frame read them
	}
	if changed {
		// Full frame cost: layout + record + raster + upload + present.
		made := wgpu.DeviceStats().Sub(devices0)
		h.core.recordFrameMade(float32(time.Since(t0).Seconds()*1000),
			h.core.prev.Len(), h.core.prev.BackdropBlurs(),
			//nolint:gosec // per-frame counts are small
			MadeCounts{
				Textures: int(made.Textures), Pipelines: int(made.Pipelines),
				Buffers: int(made.Buffers), BindGroups: int(made.BindGroups),
			})
		// GOPHICS_PACING logs a rolling frame-time summary each time the
		// 60-frame ring wraps — the on-device pacing readout (PLAN §6.4).
		if h.core.frameHead == 0 && os.Getenv("GOPHICS_PACING") != "" {
			f := h.core.FrameStats()
			log.Printf("gophics pacing: p50 %.2f  p95 %.2f  p99 %.2f  worst %.2f ms "+
				"(60 frames; worst drew %d ops / %d blurs, made %s, median %d ops)",
				f.P50, f.P95, f.P99, f.Worst, f.WorstOps, f.WorstBlurs, f.WorstMade, f.MedianOps)
		}
	}
}

// recordFrame runs the layout+record phase for one frame, recovering any panic
// from user Layout/Paint code: ok=false means the frame was dropped (logged,
// rate-limited) and nothing was recorded. Build panics are already isolated per
// subtree by safeBuild; this is the same policy for the phases that run bare.
func (h *shellHandler) recordFrame(f shell.Frame, tgt shell.Target) (changed bool, damage geom.Rect, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			h.core.framePanic(r)
			changed, damage, ok = false, geom.Rect{}, false
		}
	}()
	h.core.Layout(f.Size())
	if _, gpu := tgt.(gpuCanvasTarget); gpu {
		return h.core.RecordSceneGPU(f.Size(), f.Scale()), geom.Rect{}, true
	}
	changed, damage = h.core.RecordScene(f.Size(), f.Scale())
	return changed, damage, true
}

// framePanic logs a recovered layout/paint panic with its stack, rate-limited:
// the first occurrence always logs; repeats log at most every few seconds so a
// panic on every frame doesn't produce 60 stacks a second.
func (c *core) framePanic(r any) {
	c.framePanics++
	if c.framePanics == 1 || time.Since(c.lastPanicLog) >= 5*time.Second {
		c.lastPanicLog = time.Now()
		log.Printf("gophics: panic in layout/paint (frame dropped, %d so far, app continues): %v\n%s",
			c.framePanics, r, debug.Stack())
	}
}

func (h *shellHandler) Event(w shell.Window, e shell.Event) {
	h.window = w
	h.wireWindow(w)
	switch e := e.(type) {
	case shell.Pointer:
		h.core.Pointer(e)
		if e.Kind == shell.PointerDown {
			w.Invalidate() // start ticking for a possible long-press
		}
	case shell.Text, shell.Key, shell.Composition:
		h.core.Keyboard(e)
	case shell.Insets:
		h.core.Owner.SafeInsets = e.Insets
		h.core.Owner.RebuildAll()
		w.Invalidate()
	case shell.CapabilitiesChanged:
		// The set is not fixed at startup: a mobile host registers its backends
		// after Start, and connectivity and battery are only answerable once the
		// platform has reported them once.
		h.wireMedia(w)
		h.core.Owner.RebuildAll()
		w.Invalidate()
	case shell.KeyboardInset:
		if h.core.Owner.KeyboardInset != e.Height {
			h.core.Owner.KeyboardInset = e.Height
			h.core.Owner.RebuildAll()
			w.Invalidate()
		}
	case shell.Resize:
		w.Invalidate()
	case shell.Focus:
		// Losing focus mid-interaction (alt-tab while dragging) must not leave a
		// gesture or a held key stuck down: cancel the press/drag and clear the
		// input state.
		if !e.Focused {
			h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: -1e6, Y: -1e6}})
			if in := h.core.Owner.Input; in != nil {
				in.Clear()
			}
		}
		w.Invalidate()
	}
}
