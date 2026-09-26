package layoutbox

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// A Grid under an unbounded width — inside a Row, or a horizontal Scroll —
// has no width to divide into columns. It used cs.Min.W, usually zero, so
// every cell laid out at width 0 and the grid vanished. It shrink-wraps to
// the widest child, as Wrap does.
func TestGridShrinkWrapsUnderUnboundedWidth(t *testing.T) {
	a, b, c := &leaf{w: 30, h: 20}, &leaf{w: 50, h: 20}, &leaf{w: 10, h: 20}
	g := &Grid{Columns: 2, Spacing: 10, Children: []layout.Box{a, b, c}}
	got := g.Layout(layout.Loose(geom.Size{W: layout.Inf, H: 1000}))
	if want := (geom.Size{W: 110, H: 50}); got != want {
		t.Errorf("grid size %v, want %v (two 50-wide columns and the spacing)", got, want)
	}
	for i, ch := range g.Children {
		if w := ch.Size().W; w == 0 || w > 50 {
			t.Errorf("child %d laid out %v wide, want its own width within the 50-wide cell", i, w)
		}
	}
	if g.offsets[1].X != 60 || g.offsets[2].Y != 30 {
		t.Errorf("offsets %v: want the second cell at x=60 and the second row at y=30", g.offsets)
	}
	// A bounded width still divides it.
	if got := g.Layout(layout.Loose(geom.Size{W: 200, H: 1000})); got.W != 200 || g.offsets[1].X != 105 {
		t.Errorf("bounded: grid %v, second cell at %v; want 200 wide with 95-wide cells", got, g.offsets[1])
	}
}

// CrossStretch with an unbounded cross axis has nothing to stretch to until
// the children are measured; the flex's cross extent is then the widest
// child, and the others have to be re-laid to it. Leaving them alone made a
// stretched Column inside a horizontal scroll behave as CrossStart.
func TestFlexCrossStretchUnderUnboundedCross(t *testing.T) {
	a, b := &leaf{w: 10, h: 10}, &leaf{w: 50, h: 10}
	f := Column(a, b)
	f.CrossAlign = layout.CrossStretch
	got := f.Layout(layout.Loose(geom.Size{W: layout.Inf, H: 100}))
	if got.W != 50 {
		t.Errorf("column width %v, want 50 (the widest child)", got.W)
	}
	if a.Size().W != 50 || b.Size().W != 50 {
		t.Errorf("children %v and %v wide, want both stretched to 50", a.Size().W, b.Size().W)
	}
	if f.offsets[0].X != 0 || f.offsets[1].X != 0 {
		t.Errorf("stretched children offset %v, want both at x=0", f.offsets)
	}

	// The same for a Row with a flexed child and an unbounded height.
	c, d := &leaf{w: 10, h: 40}, &leaf{w: 10, h: 10}
	r := &Flex{Axis: layout.Horizontal, CrossAlign: layout.CrossStretch,
		Children: []FlexChild{{Box: c}, Flexible(1, d)}}
	r.Layout(layout.Loose(geom.Size{W: 100, H: layout.Inf}))
	if d.Size() != (geom.Size{W: 90, H: 40}) {
		t.Errorf("flexed child %v, want 90x40 (its share, stretched to the tallest)", d.Size())
	}
}
