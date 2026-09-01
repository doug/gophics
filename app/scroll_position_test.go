package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// twoScrollers puts two independent scroll views side by side, so "which one
// scrolled" is a question with an answer.
type twoScrollers struct{ left, right *widget.ScrollController }

func (t twoScrollers) Build(widget.Ctx) widget.Widget {
	col := func(c *widget.ScrollController, tint paint.Color) widget.Widget {
		rows := make([]widget.Widget, 40)
		for i := range rows {
			// An explicit width: a zero-width row makes the whole column
			// unhittable, and then nothing is under the pointer to scroll.
			rows[i] = widget.Sized{W: 200, H: 30, Child: widget.Decorated{Color: tint}}
		}
		return widget.Scroll{Controller: c, Child: widget.Column(rows...)}
	}
	return widget.Row(
		widget.Expand(col(t.left, paint.RGB(0.8, 0.3, 0.3))),
		widget.Expand(col(t.right, paint.RGB(0.3, 0.3, 0.8))),
	)
}

// A scroll goes to whatever is under the pointer, and the event says where
// that is — it must not depend on a pointer move having happened first.
//
// The app used to route a scroll by the last position a *move* reported, and
// the web shell threw away the coordinates the wheel event carried. Together
// that meant a wheel arriving with no preceding move over the window was
// routed from a stale position and usually scrolled nothing at all. It is the
// page that appears under a stationary cursor, and it is every scripted
// interaction.
func TestScrollGoesUnderThePointerWithoutAPriorMove(t *testing.T) {
	l, r := &widget.ScrollController{}, &widget.ScrollController{}
	h, err := NewHeadless(twoScrollers{left: l, right: r},
		Config{Size: geom.Size{W: 400, H: 300}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// No Move first: straight to a wheel over the right-hand column.
	h.ScrollAt(geom.Pt{X: 320, Y: 150}, geom.Pt{Y: -120})
	h.Render()

	if r.Offset() == 0 {
		t.Error("the column under the pointer did not scroll")
	}
	if l.Offset() != 0 {
		t.Errorf("the other column scrolled instead (offset %v)", l.Offset())
	}
}

// And the pointer position follows the scroll, so a drag begun straight after
// one starts from where the user actually is.
func TestScrollUpdatesThePointerPosition(t *testing.T) {
	l, r := &widget.ScrollController{}, &widget.ScrollController{}
	h, err := NewHeadless(twoScrollers{left: l, right: r},
		Config{Size: geom.Size{W: 400, H: 300}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	h.ScrollAt(geom.Pt{X: 320, Y: 150}, geom.Pt{Y: -60})
	h.Render()
	before := r.Offset()

	// A bare Scroll now means "at the pointer", which the ScrollAt above moved.
	h.Scroll(geom.Pt{Y: -60})
	h.Render()

	if r.Offset() == before {
		t.Error("a following scroll did not continue on the same column")
	}
	if l.Offset() != 0 {
		t.Error("it leaked to the other column")
	}
}
