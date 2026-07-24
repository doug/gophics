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

	background paint.Color
	root       widget.Widget
	size       geom.Size

	hovered []*widget.InteractiveBox
	pressed *widget.InteractiveBox
}

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
	}
	c.Owner.SetRoot(root)
	return c, nil
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

// Paint draws the current tree onto canvas (after Layout).
func (c *Core) Paint(canvas *paint.Canvas) {
	canvas.Clear(c.background)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(canvas, geom.Pt{})
	}
}

// interactivesAt returns the InteractiveBoxes under p, topmost first.
func (c *Core) interactivesAt(p geom.Pt) []*widget.InteractiveBox {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	var out []*widget.InteractiveBox
	for _, h := range layout.HitTest(box, p) {
		if ib, ok := h.Box.(*widget.InteractiveBox); ok {
			out = append(out, ib)
		}
	}
	return out
}

// Pointer dispatches a pointer event: hover enter/exit, and tap on
// press+release over the same Interactive.
func (c *Core) Pointer(e shell.Pointer) {
	switch e.Kind {
	case shell.PointerMove:
		now := c.interactivesAt(e.Pos)
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

	case shell.PointerDown:
		if e.Button != 0 {
			return
		}
		c.pressed = nil
		for _, b := range c.interactivesAt(e.Pos) {
			if b.Handler.OnTap != nil {
				c.pressed = b
				break
			}
		}

	case shell.PointerUp:
		if e.Button != 0 || c.pressed == nil {
			return
		}
		pressed := c.pressed
		c.pressed = nil
		if slices.Contains(c.interactivesAt(e.Pos), pressed) {
			pressed.Handler.OnTap()
		}
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
	h.core.Layout(f.Size())
	canvas := h.core.Painter.Begin(f)
	h.core.Paint(canvas)
	_ = h.core.Painter.End(f)
}

func (h *shellHandler) Event(w shell.Window, e shell.Event) {
	h.window = w
	switch e := e.(type) {
	case shell.Pointer:
		h.core.Pointer(e)
	case shell.Text, shell.Key:
		h.core.Keyboard(e)
	case shell.Resize:
		w.Invalidate()
	}
}
