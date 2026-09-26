package layoutbox

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// Reverse mirrors the main axis after placement, so every other rule —
// flex shares, alignment, the gaps of SpaceBetween — is computed once and
// holds in both directions.
func TestFlexReverseMirrorsPlacement(t *testing.T) {
	a, b := &leaf{w: 10, h: 10}, &leaf{w: 20, h: 10}
	r := Row(a, b)
	r.MainAlign, r.Reverse = layout.MainSpaceBetween, true
	r.Layout(layout.Tight(geom.Size{W: 100, H: 10}))
	// Forward: a at 0, b at 80. Mirrored: a at 90, b at 0.
	if r.offsets[0].X != 90 || r.offsets[1].X != 0 {
		t.Errorf("reverse space-between offsets %v, want a at 90 and b at 0", r.offsets)
	}
	c, d := &leaf{w: 10, h: 10}, &leaf{w: 10, h: 10}
	col := &Flex{Axis: layout.Vertical, Reverse: true, Children: []FlexChild{{Box: c}, Flexible(1, d)}}
	col.Layout(layout.Tight(geom.Size{W: 10, H: 100}))
	// Forward: c at 0, d fills 10..100. Mirrored: d at 0, c at 90.
	if col.offsets[0].Y != 90 || col.offsets[1].Y != 0 || d.Size().H != 90 {
		t.Errorf("reverse column offsets %v, flexed child %v; want c at 90, d at 0 and 90 tall", col.offsets, d.Size())
	}
	// Hit testing follows the mirrored placement.
	if hits := layout.HitTest(r, geom.Pt{X: 95, Y: 5}); len(hits) == 0 || hits[0].Box != a {
		t.Errorf("hit at x=95 found %v, want the first child (mirrored to the right end)", hits)
	}
}

// A Transformed box maps hits through the inverse of its transform, so a
// point inside the child's scaled-up ink but outside its layout rect still
// hits, and one inside the rect but outside the shrunk ink does not.
func TestTransformedHitTestingFollowsTheTransform(t *testing.T) {
	child := &leaf{w: 100, h: 100}
	tr := &Transformed{Child: child, Center: true}
	tr.T.SX, tr.T.SY = 2, 2
	tr.Layout(layout.Loose(geom.Size{W: 100, H: 100}))
	// Scaled 2x about the centre the child covers (-50,-50)-(150,150).
	if hits := layout.HitTest(tr, geom.Pt{X: -40, Y: -40}); len(hits) == 0 || hits[0].Box != child {
		t.Errorf("a point inside the scaled-up child missed: %v", hits)
	}
	if ink := tr.InkBounds(); ink != geom.RectXYWH(-50, -50, 200, 200) {
		t.Errorf("ink bounds %v, want (-50,-50)-(150,150)", ink)
	}

	small := &Transformed{Child: child, Center: true}
	small.T.SX, small.T.SY = 0.5, 0.5
	small.Layout(layout.Loose(geom.Size{W: 100, H: 100}))
	// Shrunk to (25,25)-(75,75): the corner of the layout rect is empty.
	if hits := layout.HitTest(small, geom.Pt{X: 5, Y: 5}); len(hits) != 0 {
		t.Errorf("a point outside the shrunk child hit: %v", hits)
	}
	if hits := layout.HitTest(small, geom.Pt{X: 50, Y: 50}); len(hits) == 0 || hits[0].Box != child {
		t.Errorf("the centre of the shrunk child missed: %v", hits)
	}

	moved := &Transformed{Child: child}
	moved.T.TX, moved.T.TY = 30, 0
	moved.Layout(layout.Loose(geom.Size{W: 100, H: 100}))
	if hits := layout.HitTest(moved, geom.Pt{X: 20, Y: 50}); len(hits) != 0 {
		t.Errorf("a point the child was translated away from hit: %v", hits)
	}
	if hits := layout.HitTest(moved, geom.Pt{X: 120, Y: 50}); len(hits) == 0 || hits[0].Box != child {
		t.Errorf("a point the child was translated onto missed: %v", hits)
	}
}
