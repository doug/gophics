//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
)

// donut builds a square ring: an outer contour and an inner one wound the
// other way, so the middle is a hole under either fill rule. Two contours is
// the smallest shape that asks the rasterizer a winding question — it is also
// every glyph with a counter, which is to say most of the alphabet.
func donut() *gg.Path {
	p := &gg.Path{}
	// Outer, clockwise.
	p.MoveTo(8, 8)
	p.LineTo(56, 8)
	p.LineTo(56, 56)
	p.LineTo(8, 56)
	p.Close()
	// Inner, counter-clockwise, so non-zero winding cancels to zero inside.
	p.MoveTo(24, 24)
	p.LineTo(24, 40)
	p.LineTo(40, 40)
	p.LineTo(40, 24)
	p.Close()
	return p
}

// coverPixels rasterizes through the tile filler and returns the coverage it
// reported, indexed [y][x].
func coverPixels(t *testing.T, rule gg.FillRule) [64][64]uint8 {
	t.Helper()
	var got [64][64]uint8
	(&TileCoverageFiller{}).FillCoverage(donut(), 64, 64, rule, nil,
		func(x, y int, coverage uint8) {
			if x < 0 || x >= 64 || y < 0 || y >= 64 {
				t.Errorf("coverage reported outside the pixmap at (%d,%d)", x, y)
				return
			}
			got[y][x] = coverage
		})
	return got
}

// TestTileFillerLeavesAMultiContourHoleEmpty is the regression test for the
// defect that four call sites used to work around, none of them with a test:
//
//	software.go      shouldUseTileRasterizer rejects >1 contour
//	software.go      Stroke forces RasterizerAnalytic
//	gpu_render_context.go    dispatchDrawsToSoftware forces it for stroked paths
//	paint/paint.go           runFor forces it for glyph-run scratch contexts
//
// They all cited the same thing: the tile filler filled the gap between
// contours solid. It did, because the winding sign was derived twice — here and
// in NewLineSegment — and the two negations cancelled, so the non-zero rule saw
// every edge as +1. Fixed in flatten.go. The guards remain as routing choices,
// unpriced rather than load bearing, and this test is what keeps the fix.
//
// This asserts the geometry rather than a golden image, because the hole is a
// mathematical fact under both fill rules and needs no oracle: the centre of a
// ring is outside the shape.
func TestTileFillerLeavesAMultiContourHoleEmpty(t *testing.T) {
	for _, c := range []struct {
		name string
		rule gg.FillRule
	}{
		{"nonzero", gg.FillRuleNonZero},
		{"evenodd", gg.FillRuleEvenOdd},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := coverPixels(t, c.rule)

			// Guard against passing vacuously: a filler that draws nothing has
			// an empty hole too, and would sail through the real assertion.
			if got[32][12] == 0 {
				t.Fatal("the ring itself drew no coverage at (12,32), so this " +
					"test is not testing the hole")
			}

			if cov := got[32][32]; cov != 0 {
				t.Errorf("centre of the hole has coverage %d, want 0: the "+
					"filler filled between the contours", cov)
			}
		})
	}
}
