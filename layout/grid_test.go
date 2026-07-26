package layout

import (
	"testing"

	"github.com/doug/gossamer/geom"
)

func TestAspectRatio(t *testing.T) {
	child := &leaf{}
	a := &AspectRatio{Ratio: 2, Child: child} // 2:1
	got := a.Layout(Loose(sz(200, 200)))
	if got != sz(200, 100) {
		t.Fatalf("aspect 2:1 in 200x200 = %v, want 200x100", got)
	}
	// Height-constrained: derive width.
	got = a.Layout(Loose(sz(200, 40)))
	if got != sz(80, 40) {
		t.Fatalf("aspect 2:1 capped at h=40 = %v, want 80x40", got)
	}
}

func TestGrid(t *testing.T) {
	kids := make([]Box, 5)
	for i := range kids {
		kids[i] = &leaf{w: 10, h: 20}
	}
	g := &Grid{Columns: 2, Spacing: 10, Children: kids}
	// width 210 → 2 cols of (210-10)/2 = 100 each.
	got := g.Layout(Loose(sz(210, 1000)))
	if g.offsets[0] != (geom.Pt{X: 0, Y: 0}) || g.offsets[1].X != 110 {
		t.Fatalf("grid offsets row0 = %v, %v", g.offsets[0], g.offsets[1])
	}
	if g.offsets[2].Y == 0 {
		t.Fatal("third child should wrap to row 2")
	}
	// 5 items, 2 cols → 3 rows of height 20 + 2 gaps of 10 = 80.
	if got.H != 80 {
		t.Fatalf("grid height = %v, want 80", got.H)
	}
}

func TestWrap(t *testing.T) {
	// Three 40-wide chips in a 100-wide wrap, spacing 10: 2 fit per run.
	kids := []Box{&leaf{w: 40, h: 20}, &leaf{w: 40, h: 20}, &leaf{w: 40, h: 20}}
	w := &Wrap{Spacing: 10, RunSpacing: 5, Children: kids}
	got := w.Layout(Loose(sz(100, 1000)))
	if w.offsets[2].Y == 0 {
		t.Fatalf("third chip should wrap; offsets=%v", w.offsets)
	}
	// two runs: 20 + 5 + 20 = 45.
	if got.H != 45 {
		t.Fatalf("wrap height = %v, want 45", got.H)
	}
}

func TestFilledExpands(t *testing.T) {
	f := &Filled{}
	if got := f.Layout(Tight(sz(120, 90))); got != sz(120, 90) {
		t.Fatalf("Filled = %v, want fill 120x90", got)
	}
}
