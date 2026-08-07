package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// overscrollHarness builds a plain vertical Scroll whose content overflows the
// viewport, and returns the live viewport so tests can read Offset (scroll
// position) and Lead (elastic overscroll displacement).
func overscrollHarness(t *testing.T) (*Headless, func() *viewportRead) {
	t.Helper()
	rows := make([]widget.Widget, 10)
	for i := range rows {
		rows[i] = widget.Sized{W: 100, H: 30}
	}
	h, err := NewHeadless(
		widget.Scroll{Child: widget.Column(rows...)},
		Config{Size: geom.Size{W: 100, H: 100}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	read := func() *viewportRead {
		vp := findViewport(h.core.Owner.RootBox())
		if vp == nil {
			t.Fatal("no viewport in tree")
		}
		return &viewportRead{offset: vp.Offset, lead: vp.Lead}
	}
	return h, read
}

type viewportRead struct{ offset, lead float32 }

// settle steps frames until the overscroll spring comes to rest (or a cap).
func settleOverscroll(h *Headless, read func() *viewportRead) {
	for i := 0; i < 240 && read().lead != 0; i++ {
		h.Step(0.016)
		h.Render()
	}
}

func TestOverscrollTopRubberBandsAndSettles(t *testing.T) {
	h, read := overscrollHarness(t)

	// At the top, dragging further down pulls into a positive (top) overscroll
	// without moving the clamped offset.
	h.Drag(geom.Pt{X: 50, Y: 20}, geom.Pt{X: 50, Y: 90})
	h.Render()
	if o := read(); o.offset != 0 {
		t.Fatalf("offset moved at top edge: %v, want 0", o.offset)
	}
	if o := read(); o.lead <= 0 {
		t.Fatalf("top drag did not rubber-band: Lead = %v, want > 0", o.lead)
	}

	// The pull is elastic: a 70px drag displaces well under 70px.
	if l := read().lead; l >= 70 {
		t.Fatalf("overscroll not elastic: Lead = %v, want < drag(70)", l)
	}

	settleOverscroll(h, read)
	if l := read().lead; l != 0 {
		t.Fatalf("top overscroll did not settle to 0: Lead = %v", l)
	}
}

func TestOverscrollBottomRubberBandsAndSettles(t *testing.T) {
	h, read := overscrollHarness(t)

	// Scroll to the bottom, then drag up past it → negative (bottom) overscroll.
	h.Move(geom.Pt{X: 50, Y: 50})
	h.Scroll(geom.Pt{Y: -1000})
	h.Render()
	if o := read(); o.offset != 200 {
		t.Fatalf("did not reach bottom: offset = %v, want 200", o.offset)
	}

	h.Drag(geom.Pt{X: 50, Y: 90}, geom.Pt{X: 50, Y: 20})
	h.Render()
	if o := read(); o.offset != 200 {
		t.Fatalf("offset moved past bottom clamp: %v, want 200", o.offset)
	}
	if o := read(); o.lead >= 0 {
		t.Fatalf("bottom drag did not rubber-band: Lead = %v, want < 0", o.lead)
	}

	settleOverscroll(h, read)
	if l := read().lead; l != 0 {
		t.Fatalf("bottom overscroll did not settle to 0: Lead = %v", l)
	}
}
