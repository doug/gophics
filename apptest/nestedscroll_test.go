package apptest_test

import (
	"math"
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// strip is a horizontal run of fixed cells, the shape of a ballot grid or a
// chip row: wider than any phone, and with a natural height of its own.
func strip(n int, cell geom.Size) widget.Widget {
	cells := make([]widget.Widget, n)
	for i := range cells {
		cells[i] = widget.Sized{W: cell.W, H: cell.H}
	}
	return widget.Semantics{Role: layout.RoleGroup, Label: "cells", Child: widget.Row(cells...)}
}

func finite(r geom.Rect) bool {
	for _, v := range []float32{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

// A horizontal Scroll inside a vertical one has no bounded height: the outer
// viewport hands its child an unbounded main axis. That used to produce an
// Inf-tall scroll (the scrollbar layer filled cs.Max, and cs.Max was Inf) which
// turned into NaN further down, so apps wrapped the inner scroll in a Sized
// with a hand-computed height. The rule now: an unbounded axis shrink-wraps
// to the content.
func TestHorizontalScrollInsideVerticalShrinkWraps(t *testing.T) {
	cell := geom.Size{W: 60, H: 40}
	a := apptest.New(t, widget.Scroll{Child: widget.Column(
		theme.Label("above"),
		widget.Semantics{Role: layout.RoleGroup, Label: "strip",
			Child: widget.Scroll{Axis: layout.Horizontal, Child: strip(10, cell)}},
		theme.Label("below"),
	)}, apptest.Size(200, 300))

	for _, n := range a.Nodes() {
		if !finite(n.Rect) {
			t.Fatalf("node %q has a non-finite rect %+v", n.Label, n.Rect)
		}
	}
	s, c := a.MustNode("strip"), a.MustNode("cells")
	if s.Rect.Dy() != cell.H || c.Rect.Dy() != cell.H {
		t.Fatalf("horizontal scroll is %v tall around %v-tall content; want %v", s.Rect.Dy(), c.Rect.Dy(), cell.H)
	}
	if s.Rect.Dx() != 200 {
		t.Errorf("horizontal scroll should still fill its bounded width: got %v", s.Rect.Dx())
	}
	// And the row below sits directly under it rather than at infinity.
	below := a.MustNode("below")
	if below.Rect.Min.Y != s.Rect.Max.Y {
		t.Errorf("label below the strip starts at %v, strip ends at %v", below.Rect.Min.Y, s.Rect.Max.Y)
	}
}

// The mirror case: a vertical Scroll whose main axis is unbounded. A
// horizontal parent still bounds the height, so the shape that actually
// produces it is a vertical scroll inside a row inside another vertical
// scroll. Same rule, same axis as the outer one — the inner column
// shrink-wraps to its content's height (the outer scroll does the scrolling)
// and its sibling in the row is unaffected.
func TestVerticalScrollWithUnboundedMainAxisShrinkWraps(t *testing.T) {
	rows := make([]widget.Widget, 5)
	for i := range rows {
		rows[i] = widget.Sized{W: 120, H: 30}
	}
	a := apptest.New(t, widget.Scroll{Child: widget.Row(
		widget.Semantics{Role: layout.RoleGroup, Label: "column",
			Child: widget.Sized{W: 120, Child: widget.Scroll{Child: widget.Semantics{
				Role: layout.RoleGroup, Label: "rows", Child: widget.Column(rows...)}}}},
		widget.Semantics{Role: layout.RoleGroup, Label: "after", Child: widget.Sized{W: 50, H: 50}},
	)}, apptest.Size(200, 100))

	for _, n := range a.Nodes() {
		if !finite(n.Rect) {
			t.Fatalf("node %q has a non-finite rect %+v", n.Label, n.Rect)
		}
	}
	col, rowsN := a.MustNode("column"), a.MustNode("rows")
	if col.Rect.Dy() != 150 || rowsN.Rect.Dy() != 150 {
		t.Fatalf("vertical scroll is %v tall around %v-tall content; want 150", col.Rect.Dy(), rowsN.Rect.Dy())
	}
	if after := a.MustNode("after"); after.Rect.Min.X != col.Rect.Max.X {
		t.Errorf("sibling after the column starts at %v, column ends at %v", after.Rect.Min.X, col.Rect.Max.X)
	}
}
