// Package app ties the widget tree to a shell: the gophics runtime.
//
// Run drives a real window; Headless drives the same core without a display
// for tests and golden images. Both share core, so behavior verified
// headless is the shipping behavior (PLAN.md principle 3).
package app

import (
	"encoding/json"
	"log"
	"os"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Config configures a gophics app.
type Config struct {
	Title string
	// AppID identifies the app for per-user storage (where ctx.Preferences()
	// persists). A stable reverse-DNS name like "com.example.tally" is
	// conventional; empty falls back to the executable's name.
	AppID string
	Size  geom.Size // initial logical window size
	// Background is the colour behind the widget tree. It is filled every
	// frame, so it shows wherever the tree paints nothing.
	//
	// It is resolved per frame against the platform's colour scheme: set
	// BackgroundDark too and the pair follows light/dark automatically. That
	// pairing exists because a single fixed colour here is a trap — the theme
	// a widget reads is chosen per frame from the same signal, so an app that
	// hardcodes a light background renders dark-theme text over it the moment
	// the viewer prefers dark, and nothing errors.
	Background paint.Color
	// BackgroundDark replaces Background while the platform reports a dark
	// colour scheme. Leave it zero to use Background in both.
	BackgroundDark paint.Color
	Font           []byte // TTF/OTF data for the default font (required for text)
	// FontFamilies registers named families (e.g. "bold", "mono"),
	// selectable per text run via widget.Text.Font / layout.RichSpan.Font.
	FontFamilies map[string][]byte
	// Debug draws box-bounds outlines over the app (Flutter's
	// debugPaintSize). Toggle at runtime via core.SetDebugPaint.
	Debug bool
	// Renderer selects the rasterization backend: Auto (default) prefers the
	// GPU with CPU fallback, GPU forces it, CPU forces the deterministic CPU
	// rasterizer. The GOPHICS_RENDERER env var overrides this at startup.
	Renderer RendererMode
}

// RendererMode selects the rasterization backend; see the Renderer* constants.
type RendererMode = shell.RendererMode

// Renderer backends (re-exported from shell for app.Config.Renderer).
const (
	RendererAuto = shell.RendererAuto
	RendererGPU  = shell.RendererGPU
	RendererCPU  = shell.RendererCPU
)

// core is the shell-independent runtime: element tree, layout, paint, and
// input dispatch. All methods run on the UI goroutine.
type core struct {
	Owner   *widget.Owner
	Painter *paint.Painter

	// LastDamage is the damage rect of the most recent frame; Skipped
	// reports whether that frame's rasterization was skipped entirely
	// (scene unchanged). Read-only stats for tests and tooling.
	LastDamage geom.Rect
	Skipped    bool

	background     paint.Color
	backgroundDark paint.Color
	root           widget.Widget
	size           geom.Size
	debugPaint     bool
	inspect        bool        // interactive widget inspector (highlights box under pointer)
	frameTimes     [60]float32 // ring of recent raster+record durations, ms
	frameHead      int

	cur, prev     *scene.List
	lastPaintSize geom.Size
	lastScale     float32
	// lastLayout is the size the tree was last laid out at; pointer-event hit
	// testing re-lays-out only when it differs or a rebuild is pending.
	lastLayout geom.Size

	// framePanics counts recovered layout/paint panics (each drops its frame);
	// lastPanicLog rate-limits their logging.
	framePanics  int
	lastPanicLog time.Time

	posted chan func()

	// hits and hoverScratch are reused across pointer events: hits backs
	// interactivesAt's result (transient — no caller retains it), hoverScratch
	// double-buffers against hovered so enter/exit diffing never compares a
	// slice to itself.
	hits         []hitInteractive
	hoverScratch []widget.GestureTarget

	hovered        []widget.GestureTarget
	pressed        widget.GestureTarget
	pressBoxes     []widget.GestureTarget // boxes that got OnPress this gesture, awaiting OnPressEnd
	longPress      widget.GestureTarget   // box eligible for long-press
	dragging       widget.GestureTarget
	dragCandidates []hitInteractive // OnDrag boxes awaiting directional commit
	dragOrigin     geom.Pt          // window origin of the dragging box at press time
	lastPos        geom.Pt
	downPos        geom.Pt
	downTouch      bool // the current press came from a touch device
	moved          bool
	pressHeld      float64 // seconds the current press has been held, unmoved
	longFired      bool
	pendingTap     widget.GestureTarget // deferred single-tap awaiting a possible double
	pendingTapPos  geom.Pt              // where the deferred tap landed (double must be nearby)
	tapElapsed     float64

	a11y *a11yTree
}

// doubleTapWindow is how long a deferred single-tap waits for a second tap.
const doubleTapWindow = 0.30

// tapSlop is the drag distance that cancels a pending tap, in logical px.
const tapSlop = 4

// debugNoDamage forces a full-surface repaint every frame (GOPHICS_NO_DAMAGE),
// bypassing damage culling — a diagnostic to isolate damage-tracking bugs.
var debugNoDamage = os.Getenv("GOPHICS_NO_DAMAGE") != ""

// longPressSeconds is how long a still press must be held to fire OnLongPress.
const longPressSeconds = 0.5

// longPressPending reports whether a time-based gesture (long-press or a
// deferred single-tap) is running — the shell keeps frames coming while it
// is so the timers advance.
func (c *core) longPressPending() bool {
	return (c.longPress != nil && !c.moved && !c.longFired) || c.pendingTap != nil
}

// TickGestures advances time-based gestures by dt seconds: fires OnLongPress
// for a held unmoved press, and flushes a deferred single-tap once the
// double-tap window elapses.
func (c *core) TickGestures(dt float64) {
	if c.longPress != nil && !c.moved && !c.longFired {
		c.pressHeld += dt
		if c.pressHeld >= longPressSeconds {
			c.longFired = true
			c.pressed = nil // long-press consumes the gesture; no tap
			if h := c.longPress.GestureHandler(); h.OnLongPress != nil {
				h.OnLongPress()
			}
		}
	}
	if c.pendingTap != nil {
		c.tapElapsed += dt
		if c.tapElapsed >= doubleTapWindow {
			tap := c.pendingTap
			c.pendingTap = nil
			if h := tap.GestureHandler(); h.OnTap != nil {
				h.OnTap()
			}
		}
	}
}

// doubleTapSlop is how far (logical px) the second tap may land from the first
// and still count as a double-tap — a double-click is at ~one spot, not across
// the widget.
const doubleTapSlop = 8

// fireTap handles a completed tap on box at pos: immediate for a plain tap; for
// a double-tap-capable box, completes a double only if the second tap is near
// the first, else defers the single.
func (c *core) fireTap(box widget.GestureTarget, pos geom.Pt) {
	h := box.GestureHandler()
	if h.OnDoubleTap == nil {
		if h.OnTap != nil {
			h.OnTap()
		}
		return
	}
	if c.pendingTap == box && near(pos, c.pendingTapPos, doubleTapSlop) {
		c.pendingTap = nil // second tap in window and place: it's a double
		h.OnDoubleTap()
		return
	}
	// A new first-tap that isn't completing the pending one as a double (a
	// different box, or the same box too far away): flush the still-pending tap's
	// OnTap now so it isn't silently dropped when we overwrite it below.
	if c.pendingTap != nil {
		if ph := c.pendingTap.GestureHandler(); ph.OnTap != nil {
			ph.OnTap()
		}
	}
	c.pendingTap, c.pendingTapPos, c.tapElapsed = box, pos, 0 // first tap: defer OnTap
}

func near(a, b geom.Pt, slop float32) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy <= slop*slop
}

// newCore builds a runtime for the given root widget.
func newCore(root widget.Widget, cfg Config) (*core, error) {
	p := paint.NewPainter()
	if cfg.Font != nil {
		if err := p.LoadFont(cfg.Font); err != nil {
			return nil, err
		}
	}
	for name, data := range cfg.FontFamilies {
		if err := p.LoadFontFamily(name, data); err != nil {
			return nil, err
		}
	}
	c := &core{
		Owner:          &widget.Owner{Painter: p, Input: input.New()},
		Painter:        p,
		background:     cfg.Background,
		backgroundDark: cfg.BackgroundDark,
		root:           root,
		size:           cfg.Size,
		cur:            &scene.List{},
		prev:           &scene.List{},
		posted:         make(chan func(), 128),
	}
	c.Owner.Post = c.Post
	c.debugPaint = cfg.Debug
	return c, nil
}

// mount builds the widget tree. Callers wire all Owner hooks (RequestFrame,
// Clipboard, OpenURL) BEFORE calling this: mounting runs Init, which can
// launch goroutines that immediately Post (e.g. a cached NetworkImage), so
// the hooks must already be in place — otherwise a background Post races
// the caller still assigning them.
func (c *core) mount() {
	// Wrap the app in an OverlayHost so any widget can show dialogs, menus,
	// and snackbars above the whole tree (widget.Overlay via Of), and in a
	// DragHost so a Draggable and a DropTarget in unrelated subtrees can find
	// each other. DragHost is inside, so a drag preview shows in the overlay.
	c.Owner.SetRoot(widget.OverlayHost{Child: widget.DragHost{Child: c.root}})
}

// Post schedules fn to run on the UI goroutine before the next frame's
// build phase (§4.6): the one safe way for background goroutines to touch
// widget state. Safe to call from any goroutine.
func (c *core) Post(fn func()) {
	c.posted <- fn
	c.Owner.RequestFrameThreadSafe()
}

// drainPosted runs pending posted work; called on the UI goroutine at the
// top of each frame.
func (c *core) drainPosted() {
	for {
		select {
		case fn := <-c.posted:
			fn()
		default:
			return
		}
	}
}

// Layout flushes pending builds and lays out the tree at the given size.
func (c *core) Layout(size geom.Size) layout.Box {
	c.size = size
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	box.Layout(layout.Tight(size))
	// A LayoutBuilder only learns its constraints during layout, then marks
	// itself dirty to rebuild its child from them. Settle those rebuilds within
	// this same frame — re-flush and re-lay-out until stable — so a responsive
	// region is never blank for a frame. Bounded to guard a pathological Build
	// whose output keeps changing the constraints it sees.
	for i := 0; i < 4 && c.Owner.NeedsBuild(); i++ {
		box = c.Owner.RootBox() // FlushBuilds picks up the LayoutBuilder rebuild
		box.Layout(layout.Tight(size))
	}
	c.lastLayout = size
	return box
}

// SetDebugPaint toggles the box-bounds debug overlay at runtime.
func (c *core) SetDebugPaint(on bool) { c.debugPaint = on }

// SetInspect toggles the interactive widget inspector: while on, the box
// under the pointer is highlighted and labeled with its type and size (like
// Flutter's widget inspector). Pairs with InspectTree for the full dump.
func (c *core) SetInspect(on bool) {
	c.inspect = on
	c.Owner.RequestFrameThreadSafe()
}

// InspectTree returns the current render tree as a flat, depth-ordered dump
// (types, rects, semantics) — the data behind a widget inspector. Call
// after a frame. Runs headless.
func (c *core) InspectTree() []layout.InspectNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layout.Inspect(box)
}

// FrameStats returns the average and worst raster+record time (ms) over the
// last frames — the honest frame-pacing readout (PLAN.md §6.4).
func (c *core) FrameStats() (avg, worst float32) {
	var sum, n float32
	for _, t := range c.frameTimes {
		if t > 0 {
			sum += t
			n++
			if t > worst {
				worst = t
			}
		}
	}
	if n > 0 {
		avg = sum / n
	}
	return avg, worst
}

func (c *core) recordFrameTime(ms float32) {
	c.frameTimes[c.frameHead] = ms
	c.frameHead = (c.frameHead + 1) % len(c.frameTimes)
}

// RecordScene records the current tree into a display list and diffs it
// against the previous frame's. It reports whether rasterization is needed
// and the (surface-clamped) damage rect. A size or scale change forces full
// damage, since the painter's retained surface is reallocated.
func (c *core) RecordScene(size geom.Size, scale float32) (changed bool, damage geom.Rect) {
	return c.recordScene(size, scale, false)
}

// RecordSceneGPU records and change-detects a frame the presenter will replay
// on the GPU. Change detection still runs (an unchanged scene lets the GPU
// present skip its full re-raster), but the damage rect a CPU present would
// need is not computed: text ops aren't measured for bounds (the expensive part
// of Diff), and the layered-scene full-damage rule doesn't apply — both exist
// only for CPU partial replay.
func (c *core) RecordSceneGPU(size geom.Size, scale float32) (changed bool) {
	changed, _ = c.recordScene(size, scale, true)
	return changed
}

// nullMeasurer satisfies scene.Measurer with zero extents — used when Diff runs
// only for its changed bool and the damage bounds are discarded (GPU present).
type nullMeasurer struct{}

func (nullMeasurer) MeasureWidthIn(string, string, float32) float32 { return 0 }
func (nullMeasurer) MetricsIn(string, float32) paint.TextMetrics    { return paint.TextMetrics{} }

// bg is the background for this frame: the dark variant when the platform
// reports a dark colour scheme and one was given, else the light one.
func (c *core) bg() paint.Color {
	if c.Owner.DarkMode && c.backgroundDark != (paint.Color{}) {
		return c.backgroundDark
	}
	return c.background
}

func (c *core) recordScene(size geom.Size, scale float32, gpu bool) (changed bool, damage geom.Rect) {
	c.cur.Reset()
	rec := c.cur.Recorder()
	surface := geom.RectFromSize(size)
	// Background as FillRect, not Clear: Clear ignores clips, which would
	// wipe retained pixels outside the damage region during partial replay.
	// The background is always opaque — the surface is retained across
	// frames, so a translucent background would ghost previous frames.
	bg := c.bg()
	bg.A = 1
	rec.FillRect(surface, bg)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(rec, geom.Pt{})
		if c.debugPaint {
			layout.DebugPaint(box, rec)
		}
		if c.inspect {
			layout.InspectOverlay(box, c.lastPos, rec, c.Painter)
		}
	}

	var m scene.Measurer = c.Painter
	if gpu {
		m = nullMeasurer{} // damage bounds are discarded; skip text measurement
	}
	damage, changed = c.cur.Diff(c.prev, m)
	if debugNoDamage && changed {
		damage = surface // debug: force full repaint to isolate damage bugs
	}
	if size != c.lastPaintSize || scale != c.lastScale {
		changed, damage = true, surface
	}
	if !gpu && c.cur.HasLayers() {
		// Transform groups can't be partially replayed: their ops are recorded
		// in a transformed coordinate space, so their bounds can't feed the
		// surface-space damage rect — repaint the whole surface this frame.
		// (Opacity groups no longer set HasLayers: they record in surface
		// coordinates and Diff computes tight damage for them.) The GPU present
		// replays the whole scene anyway, so layers don't force it to treat an
		// unchanged frame as changed.
		changed, damage = true, surface
	}
	c.lastPaintSize, c.lastScale = size, scale
	damage = damage.Intersect(surface)
	if changed && damage.IsEmpty() {
		// Changed ops with degenerate bounds: repaint everything rather
		// than nothing.
		damage = surface
	}
	if gpu {
		// The GPU path re-rasters the full surface when anything changed;
		// report that honestly in the damage stats.
		if changed {
			damage = surface
		} else {
			damage = geom.Rect{}
		}
	}
	c.cur, c.prev = c.prev, c.cur // prev now holds the current scene
	c.LastDamage, c.Skipped = damage, !changed
	return changed, damage
}

// ReplayDamaged replays the current scene clipped to the damage rect,
// culling ops that don't intersect it. Pixels outside damage are untouched
// and remain valid from the previous frame (the painter's surface is
// retained across frames).
func (c *core) ReplayDamaged(canvas paint.Canvas, damage geom.Rect) {
	canvas.PushClip(damage)
	c.prev.ReplayDamage(canvas, damage, c.Painter)
	canvas.PopClip()
}

// ReplayScene replays the most recent recorded scene in full onto canvas. The
// GPU present path rasterizes the whole frame on the GPU each frame, so it
// uses this rather than damage-culled partial replay. Call after RecordScene.
func (c *core) ReplayScene(canvas paint.Canvas) {
	c.prev.Replay(canvas)
}

// hitInteractive pairs a gesture target with the hit position in its
// local coordinates.
type hitInteractive struct {
	box   widget.GestureTarget
	local geom.Pt
}

// Semantics returns the semantics tree of the current layout (a11y
// foundation, PLAN.md §6.5). Call after a frame (or Headless.Render).
func (c *core) Semantics() []layout.SemNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layout.CollectSemantics(box)
}

// interactivesAt returns the gesture targets under p, topmost first.
// Pending rebuilds are flushed AND laid out first: hit geometry (child
// offsets, sizes) is only valid after layout, and events can arrive
// between a state change and its frame. When nothing is pending and the
// size is unchanged the layout pass is skipped outright — a pointer move
// over a clean tree costs a hit test, not a layout walk. The returned
// slice is scratch reused across calls; callers must not retain it.
func (c *core) interactivesAt(p geom.Pt) []hitInteractive {
	needsLayout := c.Owner.NeedsBuild() // check before RootBox flushes builds
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	if !c.size.IsEmpty() && (needsLayout || c.size != c.lastLayout) {
		box.Layout(layout.Tight(c.size))
		c.lastLayout = c.size
	}
	c.hits = c.hits[:0]
	for _, h := range layout.HitTest(box, p) {
		if gt, ok := h.Box.(widget.GestureTarget); ok {
			c.hits = append(c.hits, hitInteractive{gt, h.Pos})
		}
	}
	return c.hits
}

// Pointer dispatches a pointer event: hover enter/exit, drag, scroll,
// tap on press+release over the same Interactive, and tap-to-focus.
func (c *core) Pointer(e shell.Pointer) {
	if c.Owner.Input != nil {
		c.Owner.Input.HandlePointer(e)
	}
	switch e.Kind {
	case shell.PointerMove:
		delta := e.Pos.Sub(c.lastPos)
		c.lastPos = e.Pos
		if c.inspect {
			c.Owner.RequestFrameThreadSafe() // repaint so the highlight tracks
		}
		// Slop detection runs for any active press, so a move cancels a
		// pending tap or long-press even on a widget with no drag handler.
		if !c.moved && (c.pressed != nil || c.longPress != nil || len(c.dragCandidates) > 0) {
			d := e.Pos.Sub(c.downPos)
			if d.X*d.X+d.Y*d.Y > tapSlop*tapSlop {
				c.moved = true
				c.pressed = nil
				c.longPress = nil
				// A priority candidate (e.g. a text selection whose press landed
				// on text) grabs the drag regardless of depth or axis, so it
				// beats a deeper scroll. Otherwise commit to the deepest
				// candidate whose axis matches the drag's dominant direction (an
				// unconstrained one always matches), so nested
				// horizontal/vertical drags disambiguate.
				for _, h := range c.dragCandidates {
					if dp := h.box.GestureHandler().DragPriority; dp != nil && dp(c.downTouch) {
						c.dragging = h.box
						c.dragOrigin = c.downPos.Sub(h.local)
						break
					}
				}
				if c.dragging == nil {
					for _, h := range c.dragCandidates {
						if h.box.GestureHandler().DragAxis.Accepts(d.X, d.Y) {
							c.dragging = h.box
							// h.local is the box-local point at press, so the box
							// origin comes from downPos (not the current move pos).
							c.dragOrigin = c.downPos.Sub(h.local)
							break
						}
					}
				}
				c.dragCandidates = c.dragCandidates[:0]
				c.firePressEnd() // the press became a drag/scroll: end any highlight
			}
		}
		if c.moved && c.dragging != nil {
			if h := c.dragging.GestureHandler(); h.OnDrag != nil {
				// Local position via the press-time origin: drags keep
				// delivering even when the pointer leaves the box.
				h.OnDrag(e.Pos.Sub(c.dragOrigin), delta)
			}
		}
		// Build the new hover set in the scratch buffer (never aliasing
		// c.hovered — the two swap each event), diff, then swap.
		now := c.hoverScratch[:0]
		for _, h := range c.interactivesAt(e.Pos) {
			now = append(now, h.box)
		}
		for _, b := range c.hovered {
			if h := b.GestureHandler(); !slices.Contains(now, b) && h.OnExit != nil {
				h.OnExit()
			}
		}
		for _, b := range now {
			if h := b.GestureHandler(); !slices.Contains(c.hovered, b) && h.OnEnter != nil {
				h.OnEnter()
			}
		}
		c.hovered, c.hoverScratch = now, c.hovered

	case shell.PointerScroll:
		for _, h := range c.interactivesAt(c.lastPos) {
			if hd := h.box.GestureHandler(); hd.OnScroll != nil {
				hd.OnScroll(e.Scroll)
				return
			}
		}

	case shell.PointerDown:
		if e.Button != 0 {
			return
		}
		c.downPos, c.lastPos, c.moved = e.Pos, e.Pos, false
		c.downTouch = e.Source == shell.SourceTouch
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		c.dragCandidates = c.dragCandidates[:0]
		c.pressBoxes = c.pressBoxes[:0]
		c.pressHeld, c.longFired = 0, false
		hits := c.interactivesAt(e.Pos)
		for _, h := range hits {
			hd := h.box.GestureHandler()
			if hd.OnPress != nil {
				hd.OnPress(h.local)
			}
			if hd.OnPressEnd != nil {
				c.pressBoxes = append(c.pressBoxes, h.box)
			}
			if c.pressed == nil && (hd.OnTap != nil || hd.OnDoubleTap != nil) {
				c.pressed = h.box
			}
			if c.longPress == nil && hd.OnLongPress != nil {
				c.longPress = h.box
			}
			// Defer drag commitment: collect every candidate (deepest first)
			// and pick one by direction on the first slop-crossing move, so a
			// horizontal Dismissible and a vertical Scroll can nest.
			if hd.OnDrag != nil {
				c.dragCandidates = append(c.dragCandidates, h)
			}
		}
		c.focusFrom(hits)

	case shell.PointerUp:
		if e.Button != 0 {
			return
		}
		pressed, dragging := c.pressed, c.dragging
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		c.dragCandidates = c.dragCandidates[:0]
		if dragging != nil {
			if h := dragging.GestureHandler(); h.OnRelease != nil {
				h.OnRelease()
			}
		}
		if pressed != nil {
			for _, h := range c.interactivesAt(e.Pos) {
				if h.box == pressed {
					c.fireTap(pressed, e.Pos)
					break
				}
			}
		}
		c.firePressEnd() // end any press highlight, tapped or not
	}
}

// firePressEnd notifies every box that received OnPress this gesture that the
// press has concluded (up, cancel, or a drag steal), then clears the list. It
// is idempotent: the second call in a gesture finds an empty list.
func (c *core) firePressEnd() {
	for _, b := range c.pressBoxes {
		if h := b.GestureHandler(); h.OnPressEnd != nil {
			h.OnPressEnd()
		}
	}
	c.pressBoxes = c.pressBoxes[:0]
}

// focusFrom moves keyboard focus to the topmost focusable hit, if any.
// A press on nothing focusable leaves focus where it is.
func (c *core) focusFrom(hits []hitInteractive) {
	for _, hit := range hits {
		h := hit.box.GestureHandler()
		if h.OnText == nil && h.OnKey == nil {
			continue
		}
		old := c.Owner.KeyboardTarget
		if old == h {
			return
		}
		c.Owner.KeyboardTarget = h
		if old != nil && old.OnFocus != nil {
			old.OnFocus(false)
		}
		if h.OnFocus != nil {
			h.OnFocus(true)
		}
		return
	}
}

// Keyboard dispatches key/text events to the current keyboard target.
func (c *core) Keyboard(e shell.Event) {
	// Feed held-state polling first, before the focus early-return — a game
	// canvas polls keys with no focused widget.
	if in := c.Owner.Input; in != nil {
		if k, ok := e.(shell.Key); ok {
			in.HandleKey(k)
		}
		t := c.Owner.KeyboardTarget
		in.SetTextCapturing(t != nil && t.OnText != nil)
	}
	t := c.Owner.KeyboardTarget
	if t == nil {
		return
	}
	switch e := e.(type) {
	case shell.Text:
		if t.OnText != nil {
			t.OnText(e.S)
		}
	case shell.Key:
		if t.OnKey != nil {
			t.OnKey(e)
		}
	case shell.Composition:
		if t.OnComposition != nil {
			t.OnComposition(e)
		}
	}
}

// Run opens a window and runs the app until the window closes.
//
// Two environment variables adjust it, so any example doubles as a tool:
// GOPHICS_THUMB=<path> renders one frame headless to a PNG and exits (no window
// — the gallery-thumbnail hook, see thumb.go), and GOPHICS_RENDERER=auto|gpu|cpu
// overrides cfg.Renderer at startup.
func Run(root widget.Widget, cfg Config) error {
	// Gallery-thumbnail capture: when GOPHICS_THUMB is set, render the real app
	// headless to a PNG and exit — no display, no browser. See thumb.go.
	if done, err := maybeCaptureThumb(root, cfg); done {
		return err
	}

	h, err := NewHandler(root, cfg)
	if err != nil {
		return err
	}
	if sh, ok := h.(*shellHandler); ok {
		setupDevState(sh) // no-op unless running under `gophics dev`
	}
	// Resolve the renderer (env override wins) and, when CPU is selected, drop
	// the GPU accelerator so nothing offloads to it.
	renderer := shell.ResolveRenderer(cfg.Renderer)
	if renderer == shell.RendererCPU {
		paint.UseCPU()
	}
	return desktopRun(h, shell.Config{Title: cfg.Title, AppID: cfg.AppID, Size: cfg.Size, Resizable: true, Renderer: renderer})
}

// NewHandler builds the app's shell.Handler without attaching a shell —
// for embedded hosts (shell/mobile bridges) that own the surface and
// event loop.
func NewHandler(root widget.Widget, cfg Config) (shell.Handler, error) {
	core, err := newCore(root, cfg)
	if err != nil {
		return nil, err
	}
	h := &shellHandler{core: core}
	core.Owner.RequestFrame = func() {
		if h.window != nil {
			h.window.Invalidate()
		}
	}
	core.mount() // hooks wired above; safe to mount (may launch Posters)
	return h, nil
}

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

// TextInputActive reports whether a widget currently accepts keyboard
// input — embedded hosts use it to show/hide the on-screen keyboard.
func (h *shellHandler) TextInputActive() bool {
	return h.core.Owner.KeyboardTarget != nil
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
	// Frame pipeline (PLAN.md §3): posted work → tick animations → build →
	// layout → record → diff → replay damage → present.
	h.core.drainPosted()
	h.core.TickGestures(dt)
	if h.core.Owner.TickAll(dt) || h.core.longPressPending() {
		w.Invalidate() // animations or a held long-press: keep frames coming
	}
	t0 := time.Now()
	// Resolve the presentation target up front: a GPU target replays the whole
	// scene, so the damage rect (and its per-text-op measurement) is never
	// computed for it — see RecordSceneGPU.
	tgt := f.Target()
	changed, damage, ok := h.recordFrame(f, tgt)
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
		h.core.recordFrameTime(float32(time.Since(t0).Seconds() * 1000))
		// GOPHICS_PACING logs a rolling frame-time summary each time the
		// 60-frame ring wraps — the on-device pacing readout (PLAN §6.4).
		if h.core.frameHead == 0 && os.Getenv("GOPHICS_PACING") != "" {
			avg, worst := h.core.FrameStats()
			log.Printf("gophics pacing: avg %.2f ms  worst %.2f ms (60 frames)", avg, worst)
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
