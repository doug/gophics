// Package app ties the widget tree to a shell: the gophics runtime.
//
// Run drives a real window; Headless drives the same core without a display
// for tests and golden images. Both share core, so behavior verified
// headless is the shipping behavior.
package app

import (
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/internal/gfx/gg"
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
	// Provide makes values available to every widget in the app, found with
	// ctx.Of / ctx.MustOf by type, without the tree nesting a Provide for
	// each one:
	//
	//	app.Run(root{}, app.Config{Provide: []any{api, store}})
	//	...
	//	api := ctx.MustOf[API]()
	//
	// This is where an app-lifetime value belongs — an API client, a
	// database handle, a clock — the things built once at startup that never
	// vary by where you are in the tree. Values that are *derived*, like a
	// theme recomputed each build to follow the system's dark mode, belong in
	// the tree as a Provide, because they change and the subtree below them
	// has to rebuild when they do.
	//
	// Implemented by wrapping the root, so there is one lookup and one rule:
	// nearest ancestor wins. A Provide inside the tree therefore overrides
	// one from here for its own subtree, with nothing new to learn.
	//
	// Keyed by each value's dynamic type, which is what Provide[T] already
	// does — a *httpAPI here answers Of[API] for any interface it satisfies.
	// Two values satisfying the same interface is ambiguous for the same
	// reason it is in the tree; later entries sit nearer the app, so a later
	// one wins.
	Provide []any
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
	// ScrollPhysics overrides the platform's touch-fling curve. Leave it zero
	// to take the platform's: an iPhone decays one way, an Android another,
	// and a user's reference for "native" is the device in their hand. Set it
	// for an app that deliberately wants one identity everywhere — a game.
	ScrollPhysics shell.ScrollPhysics
	Renderer      RendererMode
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
	provide        []any
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
		provide:        cfg.Provide,
		root:           root,
		size:           cfg.Size,
		cur:            &scene.List{},
		prev:           &scene.List{},
		posted:         make(chan func(), 128),
	}
	c.Owner.Post = c.Post
	c.Owner.ScrollPhysics = cfg.ScrollPhysics
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
	// Config.Provide, innermost last: a later entry sits nearer the app, so it
	// wins over an earlier one of the same type, matching the tree's
	// nearest-ancestor rule.
	for i := len(c.provide) - 1; i >= 0; i-- {
		root = widget.RootProvide{Value: c.provide[i], Child: root}
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
