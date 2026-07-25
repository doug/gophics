// Package app ties the widget tree to a shell: the gossamer runtime.
//
// Run drives a real window; Headless drives the same core without a display
// for tests and golden images. Both share Core, so behavior verified
// headless is the shipping behavior (PLAN.md principle 3).
package app

import (
	"slices"

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

	cur, prev     *scene.List
	lastPaintSize geom.Size
	lastScale     float32

	posted chan func()

	hovered    []*widget.InteractiveBox
	pressed    *widget.InteractiveBox
	dragging   *widget.InteractiveBox
	dragOrigin geom.Pt // window origin of the dragging box at press time
	lastPos    geom.Pt
	downPos    geom.Pt
	moved      bool
}

// tapSlop is the drag distance that cancels a pending tap, in logical px.
const tapSlop = 4

// NewCore builds a runtime for the given root widget.
func NewCore(root widget.Widget, cfg Config) (*Core, error) {
	p := paint.NewPainter()
	if cfg.Font != nil {
		if err := p.LoadFont(cfg.Font); err != nil {
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
	c.Owner.SetRoot(root)
	return c, nil
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
	rec.FillRect(surface, c.background)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(rec, geom.Pt{})
	}

	damage, changed = c.cur.Diff(c.prev, c.Painter)
	if size != c.lastPaintSize || scale != c.lastScale {
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
func (c *Core) interactivesAt(p geom.Pt) []hitInteractive {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
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
		if c.dragging != nil {
			if !c.moved {
				d := e.Pos.Sub(c.downPos)
				if d.X*d.X+d.Y*d.Y > tapSlop*tapSlop {
					c.moved = true
					c.pressed = nil // slop exceeded: cancel pending tap
				}
			}
			if c.moved && c.dragging.Handler.OnDrag != nil {
				// Local position via the press-time origin: drags keep
				// delivering even when the pointer leaves the box.
				c.dragging.Handler.OnDrag(e.Pos.Sub(c.dragOrigin), delta)
			}
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
		c.pressed, c.dragging = nil, nil
		hits := c.interactivesAt(e.Pos)
		for _, h := range hits {
			if h.box.Handler.OnPress != nil {
				h.box.Handler.OnPress(h.local)
			}
			if c.pressed == nil && h.box.Handler.OnTap != nil {
				c.pressed = h.box
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
		c.pressed, c.dragging = nil, nil
		if dragging != nil && dragging.Handler.OnRelease != nil {
			dragging.Handler.OnRelease()
		}
		if pressed != nil && slices.Contains(boxes(c.interactivesAt(e.Pos)), pressed) {
			pressed.Handler.OnTap()
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
	core, err := NewCore(root, cfg)
	if err != nil {
		return err
	}
	h := &shellHandler{core: core}
	core.Owner.RequestFrame = func() {
		if h.window != nil {
			h.window.Invalidate()
		}
	}
	return desktopRun(h, shell.Config{Title: cfg.Title, Size: cfg.Size, Resizable: true})
}

type shellHandler struct {
	core   *Core
	window shell.Window
}

func (h *shellHandler) Frame(w shell.Window, f shell.Frame, dt float64) {
	h.window = w
	h.core.Owner.Clipboard = w
	// Frame pipeline (PLAN.md §3): posted work → tick animations → build →
	// layout → record → diff → replay damage → present.
	h.core.drainPosted()
	if h.core.Owner.TickAll(dt) {
		w.Invalidate() // animations still running: keep frames coming
	}
	h.core.Layout(f.Size())
	if changed, damage := h.core.RecordScene(f.Size(), f.Scale()); changed {
		canvas := h.core.Painter.Begin(f)
		h.core.ReplayDamaged(canvas, damage)
	}
	// Present even when skipped: the painter's surface is retained, and the
	// swapchain still needs this frame's image.
	_ = h.core.Painter.End(f)
}

func (h *shellHandler) Event(w shell.Window, e shell.Event) {
	h.window = w
	h.core.Owner.Clipboard = w
	switch e := e.(type) {
	case shell.Pointer:
		h.core.Pointer(e)
	case shell.Text, shell.Key, shell.Composition:
		h.core.Keyboard(e)
	case shell.Resize:
		w.Invalidate()
	}
}
