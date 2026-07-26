// Package app ties the widget tree to a shell: the gossamer runtime.
//
// Run drives a real window; Headless drives the same core without a display
// for tests and golden images. Both share Core, so behavior verified
// headless is the shipping behavior (PLAN.md principle 3).
package app

import (
	"log"
	"os"
	"slices"
	"time"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/scene"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// Config configures a gossamer app.
type Config struct {
	Title      string
	Size       geom.Size // initial logical window size
	Background paint.Color
	Font       []byte // TTF/OTF data for the default font (required for text)
	// FontFamilies registers named families (e.g. "bold", "mono"),
	// selectable per text run via widget.Text.Font / layout.RichSpan.Font.
	FontFamilies map[string][]byte
	// Debug draws box-bounds outlines over the app (Flutter's
	// debugPaintSize). Toggle at runtime via Core.SetDebugPaint.
	Debug bool
}

// Core is the shell-independent runtime: element tree, layout, paint, and
// input dispatch. All methods run on the UI goroutine.
type Core struct {
	Owner   *widget.Owner
	Painter *paint.Painter

	// LastDamage is the damage rect of the most recent frame; Skipped
	// reports whether that frame's rasterization was skipped entirely
	// (scene unchanged). Read-only stats for tests and tooling.
	LastDamage geom.Rect
	Skipped    bool

	background paint.Color
	root       widget.Widget
	size       geom.Size
	debugPaint bool
	frameTimes [60]float32 // ring of recent raster+record durations, ms
	frameHead  int

	cur, prev     *scene.List
	lastPaintSize geom.Size
	lastScale     float32

	posted chan func()

	hovered    []*widget.InteractiveBox
	pressed    *widget.InteractiveBox
	longPress  *widget.InteractiveBox // box eligible for long-press
	dragging   *widget.InteractiveBox
	dragOrigin geom.Pt // window origin of the dragging box at press time
	lastPos    geom.Pt
	downPos    geom.Pt
	moved      bool
	pressHeld  float64 // seconds the current press has been held, unmoved
	longFired  bool
	pendingTap *widget.InteractiveBox // deferred single-tap awaiting a possible double
	tapElapsed float64

	a11y *a11yTree
}

// doubleTapWindow is how long a deferred single-tap waits for a second tap.
const doubleTapWindow = 0.30

// tapSlop is the drag distance that cancels a pending tap, in logical px.
const tapSlop = 4

// longPressSeconds is how long a still press must be held to fire OnLongPress.
const longPressSeconds = 0.5

// longPressPending reports whether a time-based gesture (long-press or a
// deferred single-tap) is running — the shell keeps frames coming while it
// is so the timers advance.
func (c *Core) longPressPending() bool {
	return (c.longPress != nil && !c.moved && !c.longFired) || c.pendingTap != nil
}

// TickGestures advances time-based gestures by dt seconds: fires OnLongPress
// for a held unmoved press, and flushes a deferred single-tap once the
// double-tap window elapses.
func (c *Core) TickGestures(dt float64) {
	if c.longPress != nil && !c.moved && !c.longFired {
		c.pressHeld += dt
		if c.pressHeld >= longPressSeconds {
			c.longFired = true
			c.pressed = nil // long-press consumes the gesture; no tap
			if c.longPress.Handler.OnLongPress != nil {
				c.longPress.Handler.OnLongPress()
			}
		}
	}
	if c.pendingTap != nil {
		c.tapElapsed += dt
		if c.tapElapsed >= doubleTapWindow {
			tap := c.pendingTap
			c.pendingTap = nil
			if tap.Handler.OnTap != nil {
				tap.Handler.OnTap()
			}
		}
	}
}

// fireTap handles a completed tap on box: immediate for a plain tap; for a
// double-tap-capable box, either completes a double or defers the single.
func (c *Core) fireTap(box *widget.InteractiveBox) {
	if box.Handler.OnDoubleTap == nil {
		if box.Handler.OnTap != nil {
			box.Handler.OnTap()
		}
		return
	}
	if c.pendingTap == box {
		c.pendingTap = nil // second tap in window: it's a double
		box.Handler.OnDoubleTap()
		return
	}
	c.pendingTap, c.tapElapsed = box, 0 // first tap: defer OnTap
}

// NewCore builds a runtime for the given root widget.
func NewCore(root widget.Widget, cfg Config) (*Core, error) {
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
	c := &Core{
		Owner:      &widget.Owner{Painter: p},
		Painter:    p,
		background: cfg.Background,
		root:       root,
		size:       cfg.Size,
		cur:        &scene.List{},
		prev:       &scene.List{},
		posted:     make(chan func(), 128),
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
func (c *Core) mount() {
	// Wrap the app in an OverlayHost so any widget can show dialogs, menus,
	// and snackbars above the whole tree (widget.Overlay via Of).
	c.Owner.SetRoot(widget.OverlayHost{Child: c.root})
}

// Post schedules fn to run on the UI goroutine before the next frame's
// build phase (§4.6): the one safe way for background goroutines to touch
// widget state. Safe to call from any goroutine.
func (c *Core) Post(fn func()) {
	c.posted <- fn
	c.Owner.RequestFrameThreadSafe()
}

// drainPosted runs pending posted work; called on the UI goroutine at the
// top of each frame.
func (c *Core) drainPosted() {
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
func (c *Core) Layout(size geom.Size) layout.Box {
	c.size = size
	box := c.Owner.RootBox()
	if box != nil {
		box.Layout(layout.Tight(size))
	}
	return box
}

// Paint draws the current tree onto canvas (after Layout), unconditionally.
// The damage-aware path is RecordScene + ReplayDamaged.
func (c *Core) Paint(canvas paint.Canvas) {
	canvas.Clear(c.background)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(canvas, geom.Pt{})
	}
}

// SetDebugPaint toggles the box-bounds debug overlay at runtime.
func (c *Core) SetDebugPaint(on bool) { c.debugPaint = on }

// InspectTree returns the current render tree as a flat, depth-ordered dump
// (types, rects, semantics) — the data behind a widget inspector. Call
// after a frame. Runs headless.
func (c *Core) InspectTree() []layout.InspectNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layout.Inspect(box)
}

// FrameStats returns the average and worst raster+record time (ms) over the
// last frames — the honest frame-pacing readout (PLAN.md §6.4).
func (c *Core) FrameStats() (avg, worst float32) {
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

func (c *Core) recordFrameTime(ms float32) {
	c.frameTimes[c.frameHead] = ms
	c.frameHead = (c.frameHead + 1) % len(c.frameTimes)
}

// RecordScene records the current tree into a display list and diffs it
// against the previous frame's. It reports whether rasterization is needed
// and the (surface-clamped) damage rect. A size or scale change forces full
// damage, since the painter's retained surface is reallocated.
func (c *Core) RecordScene(size geom.Size, scale float32) (changed bool, damage geom.Rect) {
	c.cur.Reset()
	rec := c.cur.Recorder()
	surface := geom.RectFromSize(size)
	// Background as FillRect, not Clear: Clear ignores clips, which would
	// wipe retained pixels outside the damage region during partial replay.
	// The background is always opaque — the surface is retained across
	// frames, so a translucent background would ghost previous frames.
	bg := c.background
	bg.A = 1
	rec.FillRect(surface, bg)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(rec, geom.Pt{})
		if c.debugPaint {
			layout.DebugPaint(box, rec)
		}
	}

	damage, changed = c.cur.Diff(c.prev, c.Painter)
	if size != c.lastPaintSize || scale != c.lastScale {
		changed, damage = true, surface
	}
	if c.cur.HasLayers() {
		// Opacity groups can't be partially replayed (a culled layer
		// composites wrong), so repaint the whole surface this frame.
		changed, damage = true, surface
	}
	c.lastPaintSize, c.lastScale = size, scale
	damage = damage.Intersect(surface)
	if changed && damage.IsEmpty() {
		// Changed ops with degenerate bounds: repaint everything rather
		// than nothing.
		damage = surface
	}
	c.cur, c.prev = c.prev, c.cur // prev now holds the current scene
	c.LastDamage, c.Skipped = damage, !changed
	return changed, damage
}

// ReplayDamaged replays the current scene clipped to the damage rect,
// culling ops that don't intersect it. Pixels outside damage are untouched
// and remain valid from the previous frame (the painter's surface is
// retained across frames).
func (c *Core) ReplayDamaged(canvas paint.Canvas, damage geom.Rect) {
	canvas.PushClip(damage)
	c.prev.ReplayDamage(canvas, damage, c.Painter)
	canvas.PopClip()
}

// hitInteractive pairs an InteractiveBox with the hit position in its
// local coordinates.
type hitInteractive struct {
	box   *widget.InteractiveBox
	local geom.Pt
}

// Semantics returns the semantics tree of the current layout (a11y
// foundation, PLAN.md §6.5). Call after a frame (or Headless.Render).
func (c *Core) Semantics() []layout.SemNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layout.CollectSemantics(box)
}

// interactivesAt returns the InteractiveBoxes under p, topmost first.
// Pending rebuilds are flushed AND laid out first: hit geometry (child
// offsets, sizes) is only valid after layout, and events can arrive
// between a state change and its frame.
func (c *Core) interactivesAt(p geom.Pt) []hitInteractive {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	if !c.size.IsEmpty() {
		box.Layout(layout.Tight(c.size))
	}
	var out []hitInteractive
	for _, h := range layout.HitTest(box, p) {
		if ib, ok := h.Box.(*widget.InteractiveBox); ok {
			out = append(out, hitInteractive{ib, h.Pos})
		}
	}
	return out
}

func boxes(hits []hitInteractive) []*widget.InteractiveBox {
	out := make([]*widget.InteractiveBox, len(hits))
	for i, h := range hits {
		out[i] = h.box
	}
	return out
}

// Pointer dispatches a pointer event: hover enter/exit, drag, scroll,
// tap on press+release over the same Interactive, and tap-to-focus.
func (c *Core) Pointer(e shell.Pointer) {
	switch e.Kind {
	case shell.PointerMove:
		delta := e.Pos.Sub(c.lastPos)
		c.lastPos = e.Pos
		// Slop detection runs for any active press, so a move cancels a
		// pending tap or long-press even on a widget with no drag handler.
		if !c.moved && (c.pressed != nil || c.longPress != nil || c.dragging != nil) {
			d := e.Pos.Sub(c.downPos)
			if d.X*d.X+d.Y*d.Y > tapSlop*tapSlop {
				c.moved = true
				c.pressed = nil
				c.longPress = nil
			}
		}
		if c.moved && c.dragging != nil && c.dragging.Handler.OnDrag != nil {
			// Local position via the press-time origin: drags keep
			// delivering even when the pointer leaves the box.
			c.dragging.Handler.OnDrag(e.Pos.Sub(c.dragOrigin), delta)
		}
		now := boxes(c.interactivesAt(e.Pos))
		for _, b := range c.hovered {
			if !slices.Contains(now, b) && b.Handler.OnExit != nil {
				b.Handler.OnExit()
			}
		}
		for _, b := range now {
			if !slices.Contains(c.hovered, b) && b.Handler.OnEnter != nil {
				b.Handler.OnEnter()
			}
		}
		c.hovered = now

	case shell.PointerScroll:
		for _, h := range c.interactivesAt(c.lastPos) {
			if h.box.Handler.OnScroll != nil {
				h.box.Handler.OnScroll(e.Scroll)
				return
			}
		}

	case shell.PointerDown:
		if e.Button != 0 {
			return
		}
		c.downPos, c.lastPos, c.moved = e.Pos, e.Pos, false
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		c.pressHeld, c.longFired = 0, false
		hits := c.interactivesAt(e.Pos)
		for _, h := range hits {
			if h.box.Handler.OnPress != nil {
				h.box.Handler.OnPress(h.local)
			}
			if c.pressed == nil && (h.box.Handler.OnTap != nil || h.box.Handler.OnDoubleTap != nil) {
				c.pressed = h.box
			}
			if c.longPress == nil && h.box.Handler.OnLongPress != nil {
				c.longPress = h.box
			}
			if c.dragging == nil && h.box.Handler.OnDrag != nil {
				c.dragging = h.box
				c.dragOrigin = e.Pos.Sub(h.local)
			}
		}
		c.focusFrom(hits)

	case shell.PointerUp:
		if e.Button != 0 {
			return
		}
		pressed, dragging := c.pressed, c.dragging
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		if dragging != nil && dragging.Handler.OnRelease != nil {
			dragging.Handler.OnRelease()
		}
		if pressed != nil && slices.Contains(boxes(c.interactivesAt(e.Pos)), pressed) {
			c.fireTap(pressed)
		}
	}
}

// focusFrom moves keyboard focus to the topmost focusable hit, if any.
// A press on nothing focusable leaves focus where it is.
func (c *Core) focusFrom(hits []hitInteractive) {
	for _, hit := range hits {
		h := &hit.box.Handler
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
func (c *Core) Keyboard(e shell.Event) {
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
func Run(root widget.Widget, cfg Config) error {
	h, err := NewHandler(root, cfg)
	if err != nil {
		return err
	}
	return desktopRun(h, shell.Config{Title: cfg.Title, Size: cfg.Size, Resizable: true})
}

// NewHandler builds the app's shell.Handler without attaching a shell —
// for embedded hosts (shell/mobile bridges) that own the surface and
// event loop.
func NewHandler(root widget.Widget, cfg Config) (shell.Handler, error) {
	core, err := NewCore(root, cfg)
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
	core   *Core
	window shell.Window
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
	h.core.Owner.Clipboard = w
	h.core.Owner.OpenURL = w.OpenURL
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
	h.core.Layout(f.Size())
	changed, damage := h.core.RecordScene(f.Size(), f.Scale())
	if changed {
		canvas := h.core.Painter.Begin(f)
		h.core.ReplayDamaged(canvas, damage)
	}
	// Present even when skipped: the painter's surface is retained, and the
	// swapchain still needs this frame's image.
	if err := h.core.Painter.End(f); err != nil {
		log.Printf("gossamer: present: %v", err)
	}
	if changed {
		// Full frame cost: layout + record + raster + upload + present.
		h.core.recordFrameTime(float32(time.Since(t0).Seconds() * 1000))
		// GOSSAMER_PACING logs a rolling frame-time summary each time the
		// 60-frame ring wraps — the on-device pacing readout (PLAN §6.4).
		if h.core.frameHead == 0 && os.Getenv("GOSSAMER_PACING") != "" {
			avg, worst := h.core.FrameStats()
			log.Printf("gossamer pacing: avg %.2f ms  worst %.2f ms (60 frames)", avg, worst)
		}
	}
}

func (h *shellHandler) Event(w shell.Window, e shell.Event) {
	h.window = w
	h.core.Owner.Clipboard = w
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
	case shell.Resize:
		w.Invalidate()
	}
}
