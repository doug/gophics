package layout

import (
	"testing"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// leaf is a test box that wants a fixed size.
type leaf struct {
	Base
	w, h float32
}

func (l *leaf) Layout(cs Constraints) geom.Size {
	return l.setSize(cs.Constrain(geom.Size{W: l.w, H: l.h}))
}
func (l *leaf) Paint(c paint.Canvas, at geom.Pt) {}
func (l *leaf) AddHits(p geom.Pt, hits *[]Hit) {
	if l.contains(p) {
		*hits = append(*hits, Hit{l, p})
	}
}

func sz(w, h float32) geom.Size { return geom.Size{W: w, H: h} }

func TestConstraints(t *testing.T) {
	c := Constraints{Min: sz(10, 10), Max: sz(100, 100)}
	if got := c.Constrain(sz(5, 500)); got != sz(10, 100) {
		t.Fatalf("Constrain = %v", got)
	}
	if got := c.Loosen(); got.Min != (geom.Size{}) || got.Max != sz(100, 100) {
		t.Fatalf("Loosen = %v", got)
	}
	d := c.Deflate(geom.InsetsAll(10))
	if d.Min != sz(0, 0) || d.Max != sz(80, 80) {
		t.Fatalf("Deflate = %v", d)
	}
	if !Unbounded().Constrain(sz(1e9, 1e9)).IsEmpty() == false {
		t.Fatal("unbounded should admit any size")
	}
	if Tight(sz(50, 50)).Constrain(sz(0, 999)) != sz(50, 50) {
		t.Fatal("tight constraints force the size")
	}
}

func TestPadded(t *testing.T) {
	child := &leaf{w: 50, h: 20}
	p := &Padded{Insets: geom.InsetsAll(10), Child: child}
	got := p.Layout(Loose(sz(200, 200)))
	if got != sz(70, 40) {
		t.Fatalf("Padded size = %v, want 70x40", got)
	}
	hits := HitTest(p, geom.Pt{X: 15, Y: 15})
	if len(hits) != 2 || hits[0].Box != Box(child) {
		t.Fatalf("hit path = %v, want child first", hits)
	}
	if hits[0].Pos != (geom.Pt{X: 5, Y: 5}) {
		t.Fatalf("child-local pos = %v, want (5,5)", hits[0].Pos)
	}
	if len(HitTest(p, geom.Pt{X: 5, Y: 5})) != 1 {
		t.Fatal("padding area should hit only the Padded box")
	}
}

func TestAlignedCenters(t *testing.T) {
	child := &leaf{w: 20, h: 10}
	a := Center(child)
	if got := a.Layout(Tight(sz(100, 100))); got != sz(100, 100) {
		t.Fatalf("Aligned should expand, got %v", got)
	}
	hits := HitTest(a, geom.Pt{X: 50, Y: 50})
	if len(hits) != 2 {
		t.Fatalf("center point should hit child, path = %v", hits)
	}
	if len(HitTest(a, geom.Pt{X: 5, Y: 5})) != 1 {
		t.Fatal("corner should miss the centered child")
	}
}

func TestSized(t *testing.T) {
	s := &Sized{W: 30, Child: &leaf{w: 5, h: 5}}
	got := s.Layout(Loose(sz(100, 100)))
	if got != sz(30, 5) {
		t.Fatalf("Sized = %v, want 30x5 (W forced, H from child)", got)
	}
}

func TestFlexRowFixedAndFlexible(t *testing.T) {
	a := &leaf{w: 30, h: 10}
	b := &leaf{w: 0, h: 20} // flexible: gets remaining width
	c := &leaf{w: 50, h: 10}
	f := &Flex{Axis: Horizontal, CrossAlign: CrossCenter, Children: []FlexChild{
		{Box: a}, Flexible(1, b), {Box: c},
	}}
	got := f.Layout(Tight(sz(200, 40)))
	if got != sz(200, 40) {
		t.Fatalf("flex row size = %v, want tight 200x40", got)
	}
	if b.Size().W != 120 {
		t.Fatalf("flexible width = %v, want 120 (200-30-50)", b.Size().W)
	}
	// Offsets: a at 0, b at 30, c at 150; cross-centered.
	if f.offsets[1] != (geom.Pt{X: 30, Y: 10}) {
		t.Fatalf("flexible offset = %v, want (30,10)", f.offsets[1])
	}
	if f.offsets[2] != (geom.Pt{X: 150, Y: 15}) {
		t.Fatalf("last offset = %v, want (150,15)", f.offsets[2])
	}
}

func TestFlexWeights(t *testing.T) {
	a, b := &leaf{}, &leaf{}
	f := &Flex{Axis: Horizontal, Children: []FlexChild{Flexible(1, a), Flexible(3, b)}}
	f.Layout(Tight(sz(100, 10)))
	if a.Size().W != 25 || b.Size().W != 75 {
		t.Fatalf("weighted split = %v/%v, want 25/75", a.Size().W, b.Size().W)
	}
}

func TestColumnShrinkWrapsUnboundedMain(t *testing.T) {
	f := Column(&leaf{w: 10, h: 30}, &leaf{w: 20, h: 40})
	got := f.Layout(Loose(geom.Size{W: 100, H: Inf}))
	if got != sz(20, 70) {
		t.Fatalf("column size = %v, want 20x70 (shrink-wrap)", got)
	}
}

func TestFlexMainAlignEnd(t *testing.T) {
	a := &leaf{w: 30, h: 10}
	f := Row(a)
	f.MainAlign = MainEnd
	f.Layout(Tight(sz(100, 10)))
	if f.offsets[0].X != 70 {
		t.Fatalf("MainEnd offset = %v, want 70", f.offsets[0].X)
	}
}

func TestHitOrderTopmostFirst(t *testing.T) {
	// Two overlapping children in a row cannot overlap, so use Aligned
	// stacking via Decorated wrapping: child inside decorated inside padded.
	inner := &leaf{w: 10, h: 10}
	dec := &Decorated{Child: inner}
	got := dec.Layout(Loose(sz(50, 50)))
	if got != sz(10, 10) {
		t.Fatalf("decorated wraps child, got %v", got)
	}
	hits := HitTest(dec, geom.Pt{X: 5, Y: 5})
	if len(hits) != 2 || hits[0].Box != Box(inner) || hits[1].Box != Box(dec) {
		t.Fatalf("hit order = %v, want inner then decorated", hits)
	}
}
