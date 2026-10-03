package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// fakeCursor records the shapes the runner asks for.
type fakeCursor struct {
	shapes []shell.CursorShape
	sets   int
}

func (c *fakeCursor) Set(s shell.CursorShape) {
	c.sets++
	if n := len(c.shapes); n == 0 || c.shapes[n-1] != s {
		c.shapes = append(c.shapes, s)
	}
}

func (c *fakeCursor) last() shell.CursorShape {
	if len(c.shapes) == 0 {
		return shell.CursorShape(255)
	}
	return c.shapes[len(c.shapes)-1]
}

// cursorWindow publishes only the cursor capability, the way a desktop or web
// window does.
type cursorWindow struct {
	shell.Window
	c shell.Cursor
}

func (w cursorWindow) Cursor() shell.Cursor { return w.c }

// cursorApp puts selectable text beside a plain, cursor-less box so the
// pointer can be moved between a region that asks for a shape and one that
// does not.
type cursorApp struct{}

func (cursorApp) CreateState() widget.State { return &cursorState{} }

type cursorState struct{ widget.StateBase[cursorApp] }

func (s *cursorState) Build(widget.Ctx) widget.Widget {
	col := widget.Column(
		widget.Sized{W: 200, H: 40, Child: widget.Fill{}}, // no cursor asked for
		widget.SelectionArea{Child: widget.Text{Value: "selectable prose", Size: 14}},
	)
	col.CrossAlign = layout.CrossStart
	return col
}

func cursorHarness(t *testing.T) (*Headless, *fakeCursor) {
	t.Helper()
	h, err := NewHeadless(cursorApp{}, Config{Size: geom.Size{W: 300, H: 120}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := &fakeCursor{}
	h.core.Owner.WireCapabilities(cursorWindow{c: c})
	h.Render()
	return h, c
}

// The pointer takes the I-beam over text and the arrow back off it, which is
// how a reader knows the text can be worked with before touching it.
func TestCursorFollowsTheTextUnderThePointer(t *testing.T) {
	h, c := cursorHarness(t)

	h.Move(geom.Pt{X: 20, Y: 50}) // over the prose
	if got := c.last(); got != shell.CursorText {
		t.Fatalf("cursor over text = %v, want text", got)
	}
	h.Move(geom.Pt{X: 20, Y: 10}) // over the plain box
	if got := c.last(); got != shell.CursorDefault {
		t.Fatalf("cursor off the text = %v, want default", got)
	}
	h.Move(geom.Pt{X: 30, Y: 50}) // back over the prose
	if got := c.last(); got != shell.CursorText {
		t.Fatalf("cursor back over text = %v, want text", got)
	}
}

// A platform with no pointer to shape publishes no capability, and the runner
// must not reach for it.
func TestCursorAbsentIsHarmless(t *testing.T) {
	h, err := NewHeadless(cursorApp{}, Config{Size: geom.Size{W: 300, H: 120}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Move(geom.Pt{X: 20, Y: 50}) // would panic if the runner assumed a cursor
}
