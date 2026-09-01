// Package app ties the widget tree to a shell: the gophics runtime.
//
// Run drives a real window; Headless drives the same core without a display
// for tests and golden images. Both share core, so behavior verified
// headless is the shipping behavior.
package app

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
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
	// EdgeToEdge stops the runner from insetting the root for the notch, the
	// status bar and the home indicator.
	//
	// The default is to inset, because forgetting is the common failure and it
	// is invisible on a desktop: four of the five examples here shipped with
	// their titles under the Dynamic Island, and the one that did not was the
	// one whose author had met the problem before. On a platform with no
	// obstructions the insets are zero and this costs nothing either way.
	//
	// Set it when the content is meant to run under the hardware — a
	// full-bleed camera preview, a photo viewer, a map — and apply
	// widget.SafeArea yourself around the parts that should not.
	EdgeToEdge bool
	// ScaleToFit treats Size as a fixed design size: the app lays out at
	// exactly Size and the shell scales the result to fit the display,
	// letterboxed, preserving aspect ratio. Set it for a layout that does not
	// reflow -- a fixed grid of pads, a board -- so it shrinks whole on a small
	// screen instead of being cropped. Leave it false for anything responsive.
	ScaleToFit bool
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
	// debugPaintSize). Toggle at runtime via Headless.SetDebugPaint, or on a
	// live app through the handler returned by NewHandler.
	Debug bool
	// Transparent clears to a translucent background instead of forcing the
	// background opaque, for a UI composited over host content — a HUD over a
	// game, an overlay window.
	//
	// It costs the retained surface. The background normally goes down as a
	// blended FillRect over pixels kept from the previous frame, so a
	// translucent one composites over stale content and ghosts; the only way to
	// make translucency correct is to replay the whole scene every frame. That
	// is a real cost, and a small one in the case this serves: an overlay host
	// is redrawing its frame and re-compositing every tick anyway, so damage
	// tracking was buying it little.
	Transparent bool
	// Renderer selects the rasterization backend: Auto (default) prefers the
	// GPU with CPU fallback, GPU forces it, CPU forces the deterministic CPU
	// rasterizer. The GOPHICS_RENDERER env var overrides this at startup.
	Renderer RendererMode
	// GraphicsLog receives diagnostics from the rendering stack: adapter and
	// surface selection, shader and pipeline compilation, atlas uploads, and
	// every fallback taken when something is unsupported. Nil, the default,
	// discards them.
	//
	// The stack is silent by default because a library should not write to a
	// program's output uninvited, and that is the right default. It has a cost
	// worth knowing: when GPU text came up blank on an Adreno tablet, the
	// render session was already logging which batches it was skipping, and
	// there was no way to hear it — the investigation started by adding this
	// wiring by hand.
	//
	// GOPHICS_GPU_LOG=debug|info|warn turns the same diagnostics on without
	// touching code, routed through the standard logger so they reach stderr
	// on desktop and logcat on Android. That matters on mobile, where the
	// process is started by the host and there is often nowhere to set a field.
	GraphicsLog *slog.Logger
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
	edgeToEdge     bool
	root           widget.Widget
	size           geom.Size
	debugPaint     bool
	transparent    bool        // Config.Transparent: translucent bg, no surface retention
	inspect        bool        // interactive widget inspector (highlights box under pointer)
	frameTimes     [60]float32 // ring of recent raster+record durations, ms
	// frameOps/frameBlurs record what each of those frames drew. A frame time
	// on its own says a frame was slow; paired with the size of the scene it
	// drew, it says whether the scene was bigger or the same scene cost more —
	// which is the difference between a heavy page and a discrete event like a
	// layer resolve or an atlas growth.
	frameOps   [60]int32
	frameBlurs [60]int32
	// frameMade records GPU resources created during each frame, by kind. A
	// spike on a median-sized scene is work that happens once, the first time
	// something is needed, and this is what makes it visible.
	//
	// By kind rather than as a total, because the two kinds mean opposite
	// things. Textures and pipelines are first-use costs that amortize away;
	// buffers and bind groups are recreated every frame by the stencil tier,
	// six per path, and never amortize.
	// "Made 312 objects" and "made 312 buffers and bind groups" are different
	// diagnoses, and the total could not tell them apart.
	frameMade [60]MadeCounts
	frameHead int

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

// tapSlop is the movement that cancels a pending tap or long-press, in logical
// px, for a pointer that holds still — a mouse or a pen.
const tapSlop = 4

// touchTapSlop is the same for a finger, which does not.
//
// One threshold used to govern both. Four px is right for a mouse and far too
// tight for a finger: a finger rolls a few points through any deliberate tap
// and drifts further through a half-second hold, so taps went unnoticed and
// long-presses were cancelled before they could fire. The symptoms do not look
// related — rows that ignore being tapped, and text selection that never
// starts, since its entry point on touch *is* a long press — which is what
// made it hard to see as one cause.
//
// Ten points is about what the platforms allow: UIKit's long-press
// allowableMovement defaults to 10, and Android's touch slop is around 8dp.
const touchTapSlop = 10

// slop is the movement this gesture is allowed before it stops being a tap.
func (c *core) slop() float32 {
	if c.downTouch {
		return touchTapSlop
	}
	return tapSlop
}

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
		edgeToEdge:     cfg.EdgeToEdge,
		root:           root,
		size:           cfg.Size,
		cur:            &scene.List{},
		prev:           &scene.List{},
		posted:         make(chan func(), 128),
	}
	c.Owner.Post = c.Post
	c.debugPaint = cfg.Debug
	c.transparent = cfg.Transparent
	applyGraphicsLog(cfg.GraphicsLog)
	return c, nil
}

// applyGraphicsLog points the rendering stack at a logger. One call reaches all
// of it: gg propagates to its GPU accelerator, which propagates to wgpu and the
// HAL beneath it, in either order — registering an accelerator later re-applies
// whatever logger is current.
//
// With nothing configured this does nothing at all, rather than resetting the
// stack to silent. The distinction matters: gg is silent to begin with, so an
// app that configures nothing gets nothing, and an embedder or test that called
// gg.SetLogger directly keeps what it asked for instead of having it cleared by
// the next app that happens to start.
func applyGraphicsLog(l *slog.Logger) {
	if l == nil {
		l = envGraphicsLog()
	}
	if l == nil {
		return
	}
	gg.SetLogger(l)
}

// envGraphicsLog builds a logger from GOPHICS_GPU_LOG, or returns nil when it
// is unset or unrecognised.
//
// Output goes through the standard logger rather than straight to stderr,
// because that is what reaches the platform's own log: gomobile routes it to
// logcat, and on desktop it lands on stderr either way.
func envGraphicsLog() *slog.Logger {
	var level slog.Level
	switch strings.ToLower(os.Getenv("GOPHICS_GPU_LOG")) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil
	}
	return slog.New(slog.NewTextHandler(stdlogWriter{}, &slog.HandlerOptions{Level: level}))
}

// stdlogWriter forwards a slog handler's output to the standard logger.
type stdlogWriter struct{}

func (stdlogWriter) Write(p []byte) (int, error) {
	// Depth 0: the standard logger's own file/line would point here, which
	// tells the reader nothing. The record already carries its source.
	_ = log.Output(0, "gophics/gpu: "+strings.TrimRight(string(p), "\n"))
	return len(p), nil
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
	// SafeArea sits inside the hosts, not around them: an overlay places
	// itself against the whole window on purpose — a dialog centres in it, a
	// snackbar decides for itself whether to clear the home indicator — while
	// the app's own tree is what must not hide under the hardware.
	var root widget.Widget = c.root
	if !c.edgeToEdge {
		root = widget.SafeArea{Child: root}
	}
	c.Owner.SetRoot(widget.OverlayHost{Child: widget.DragHost{Child: root}})
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
func (c *core) InspectTree() []layoutbox.InspectNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layoutbox.Inspect(box)
}

// FrameStats summarises recent frame times (ms) as percentiles.
//
// Percentiles rather than a mean, because a mean answers the wrong question.
// Stutter is a handful of frames far above the rest, and averaging them into
// the sixty good ones around them is exactly how they stop being visible in
// the number while staying visible on the screen. p50 says what the frame
// normally costs; p99 and the worst say what is being felt.
func (c *core) FrameStats() (s FrameSummary) {
	var buf [len(c.frameTimes)]float32
	n, worstAt := 0, -1
	for i, t := range c.frameTimes {
		if t <= 0 {
			continue
		}
		buf[n] = t
		n++
		if worstAt < 0 || t > c.frameTimes[worstAt] {
			worstAt = i
		}
	}
	if n == 0 {
		return s
	}
	sort.Slice(buf[:n], func(i, j int) bool { return buf[i] < buf[j] })
	at := func(f float32) float32 { return buf[int(f*float32(n-1))] }
	s.P50, s.P95, s.P99, s.Worst = at(0.50), at(0.95), at(0.99), buf[n-1]
	// The worst frame's own scene, not the window's: the question a spike
	// raises is what *that* frame was drawing.
	s.WorstOps, s.WorstBlurs = int(c.frameOps[worstAt]), int(c.frameBlurs[worstAt])
	s.WorstMade = c.frameMade[worstAt]
	// The median scene size, to compare the worst frame against.
	var ops [len(c.frameOps)]int32
	m := 0
	for i, t := range c.frameTimes {
		if t > 0 {
			ops[m] = c.frameOps[i]
			m++
		}
	}
	sort.Slice(ops[:m], func(i, j int) bool { return ops[i] < ops[j] })
	s.MedianOps = int(ops[m/2])
	return s
}

// FrameSummary is a window of frame times with what the worst frame drew.
//
// The ops counts are the point: a spike beside a median-sized scene is a
// discrete event — a layer resolved, an atlas grown, a glyph rasterized for
// the first time — where a spike beside a much larger scene is simply a
// heavier frame. Reporting the time alone cannot tell those apart, which is
// what made "occasional stutter" hard to act on.
type FrameSummary struct {
	P50, P95, P99, Worst float32
	WorstOps, WorstBlurs int
	MedianOps            int
	// WorstMade is what GPU resources the worst frame had to create, by kind.
	WorstMade MadeCounts
}

// MadeCounts is the GPU resources one frame created, by kind.
type MadeCounts struct {
	Textures   int
	Pipelines  int
	Buffers    int
	BindGroups int
}

// Total is every kind together.
func (m MadeCounts) Total() int { return m.Textures + m.Pipelines + m.Buffers + m.BindGroups }

// String renders the breakdown for a log line, naming only the kinds that are
// non-zero — a frame that made nothing but buffers should say so in as many
// words, not bury it in three zeroes.
func (m MadeCounts) String() string {
	if m.Total() == 0 {
		return "0 gpu objects"
	}
	parts := make([]string, 0, 4)
	for _, k := range []struct {
		n         int
		one, many string
	}{
		{m.Buffers, "buffer", "buffers"}, {m.BindGroups, "bind group", "bind groups"},
		{m.Textures, "texture", "textures"}, {m.Pipelines, "pipeline", "pipelines"},
	} {
		if k.n == 1 {
			parts = append(parts, "1 "+k.one)
		} else if k.n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", k.n, k.many))
		}
	}
	return strings.Join(parts, " + ")
}

func (c *core) recordFrameTime(ms float32) { c.recordFrame(ms, 0, 0) }

func (c *core) recordFrame(ms float32, ops, blurs int) {
	c.recordFrameMade(ms, ops, blurs, MadeCounts{})
}

func (c *core) recordFrameMade(ms float32, ops, blurs int, made MadeCounts) {
	c.frameTimes[c.frameHead] = ms
	c.frameOps[c.frameHead] = int32(ops)     //nolint:gosec // scene sizes are small
	c.frameBlurs[c.frameHead] = int32(blurs) //nolint:gosec
	c.frameMade[c.frameHead] = made
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
	//
	// Opaque unless the app asked otherwise: the surface is retained across
	// frames, so a translucent background composites over the previous frame
	// and ghosts. Config.Transparent opts into translucency and turns retention
	// off to pay for it.
	bg := c.bg()
	if !c.transparent {
		bg.A = 1
	}
	rec.FillRect(surface, bg)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(rec, geom.Pt{})
		if c.debugPaint {
			layoutbox.DebugPaint(box, rec)
		}
		if c.inspect {
			layoutbox.InspectOverlay(box, c.lastPos, rec, c.Painter)
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
	if c.transparent && changed {
		// Translucency and partial replay are incompatible: a blended
		// background over pixels kept from the previous frame ghosts it. A
		// changed frame is replayed whole.
		damage = surface
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
// foundation). Call after a frame (or Headless.Render).
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
			if sl := c.slop(); d.X*d.X+d.Y*d.Y > sl*sl {
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
						hd := h.box.GestureHandler()
						// A handler that does not want this gesture steps
						// aside rather than winning it on depth and then
						// ignoring it (see Handler.DragClaims).
						if dc := hd.DragClaims; dc != nil && !dc(c.downTouch) {
							continue
						}
						if hd.DragAxis.Accepts(d.X, d.Y) {
							c.dragging = h.box
							// h.local is the box-local point at press, so the box
							// origin comes from downPos (not the current move pos).
							c.dragOrigin = c.downPos.Sub(h.local)
							break
						}
					}
				}
				c.dragCandidates = c.dragCandidates[:0]
				// The press became a drag or a scroll: end any highlight it
				// put up — except on the box that won the drag, whose press
				// has not ended at all. It is now dragging and will get
				// OnRelease; telling it the press ended here would have it
				// tear down the gesture on the very move that started it. It
				// stays on the list so it still hears about pointer-up.
				c.firePressEndExcept(c.dragging)
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
		// Route by where the pointer is *now*. Every shell sets Pos on a
		// scroll; using the last position a *move* happened to report meant a
		// wheel that arrived without a preceding move went to whatever was
		// under the stale position, which is usually nothing. Keeping lastPos
		// in step also means a drag starting right after a scroll begins from
		// the right place.
		c.lastPos = e.Pos
		for _, h := range c.interactivesAt(e.Pos) {
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
func (c *core) firePressEnd() { c.firePressEndExcept(nil) }

// firePressEndExcept is firePressEnd with one box held back — the one that has
// just taken the drag, which is retained for the end of the gesture rather
// than notified now.
func (c *core) firePressEndExcept(skip widget.GestureTarget) {
	kept := c.pressBoxes[:0]
	for _, b := range c.pressBoxes {
		if skip != nil && b == skip {
			kept = append(kept, b) // still owed an OnPressEnd at pointer-up
			continue
		}
		if h := b.GestureHandler(); h.OnPressEnd != nil {
			h.OnPressEnd()
		}
	}
	c.pressBoxes = kept
}

// focusFrom moves keyboard focus to the topmost focusable hit.
//
// A press that hits nothing focusable releases a *text* target, and keeps any
// other. That asymmetry is deliberate. On a phone the soft keyboard is raised
// by a focused field and dismissed by it losing focus, so a rule that never
// released focus left no way to put the keyboard away at all: it covered half
// the screen until the app was closed. Tapping outside a field to dismiss is
// also what both the web and the platforms do.
//
// It is narrowed to targets that capture text (OnText) so that a widget which
// only reads keys — the edit menu, a key-driven canvas — keeps focus when the
// user presses elsewhere. Nothing about those involves a keyboard that needs
// dismissing.
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

	// Nothing focusable under the press.
	if old := c.Owner.KeyboardTarget; old != nil && old.OnText != nil {
		c.Owner.KeyboardTarget = nil
		if old.OnFocus != nil {
			old.OnFocus(false)
		}
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

	// Tab moves focus, before the focused widget sees it.
	//
	// Ahead of the dispatch because traversal is the app's business, not any
	// one widget's: a field cannot know what comes after it. A widget that
	// wants Tab for itself says so with ConsumesTab — a multiline field
	// indents — and keeps it. With nothing focused Tab still moves, so Tab
	// into a screen works like Tab within one.
	if k, ok := e.(shell.Key); ok && k.Code == shell.KeyTab && k.Kind == shell.KeyPress {
		if t == nil || t.ConsumesTab == nil || !t.ConsumesTab() {
			if c.Owner.MoveFocus(k.Mods&shell.ModShift == 0) {
				c.Owner.RebuildAll()
			}
			return
		}
	}

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
	return desktopRun(h, shell.Config{Title: cfg.Title, AppID: cfg.AppID, Size: cfg.Size, ScaleToFit: cfg.ScaleToFit, Resizable: true, Renderer: renderer})
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
