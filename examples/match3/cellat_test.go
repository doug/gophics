package main

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// TestCellAtRejectsOffBoard guards the Floor fix: taps in the band just left of
// or above the board (e.g. the score panel) must map to no cell, not row/col 0.
func TestCellAtRejectsOffBoard(t *testing.T) {
	g := &game{size: geom.Size{W: 480, H: 760}}
	b := g.layout()

	if got := g.cellAt(geom.Pt{X: b.x + 10, Y: b.y - 6}); got != noCell {
		t.Errorf("tap in the score panel above the board mapped to %v, want noCell", got)
	}
	if got := g.cellAt(geom.Pt{X: b.x - 5, Y: b.y + 10}); got != noCell {
		t.Errorf("tap just left of the board mapped to %v, want noCell", got)
	}
	// A genuine in-board tap still resolves to the right cell.
	if got := g.cellAt(geom.Pt{X: b.x + b.cell*1.5, Y: b.y + b.cell*2.5}); got != (cell{2, 1}) {
		t.Errorf("in-board tap = %v, want {r:2,c:1}", got)
	}
}
