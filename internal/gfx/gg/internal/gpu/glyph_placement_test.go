//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/text"
)

// A hinted glyph must land on a whole device pixel. The outline was grid-fit
// in both axes; drawing it off that grid resamples it into a softer, heavier
// shape, and because the mask cache quantizes the fraction to quarters,
// neighbouring glyphs pick up visibly different weights inside one word.
func TestHintedGlyphsSnapToWholeDevicePixels(t *testing.T) {
	for _, deviceScale := range []float64{1, 2, 3} {
		for _, absX := range []float64{0, 0.1, 0.33, 0.5, 0.67, 12.49, 12.51, 103.3} {
			absX, absY, fracX, fracY := glyphPlacement(absX, 40.2, deviceScale, text.HintingFull)
			if fracX != 0 || fracY != 0 {
				t.Errorf("scale %.0f x=%.2f: fracX=%v fracY=%v, want 0 for a hinted glyph",
					deviceScale, absX, fracX, fracY)
			}
			for _, v := range []struct {
				name string
				dev  float64
			}{{"x", absX * deviceScale}, {"y", absY * deviceScale}} {
				if math.Abs(v.dev-math.Round(v.dev)) > 1e-9 {
					t.Errorf("scale %.0f: device %s = %v, not a whole pixel", deviceScale, v.name, v.dev)
				}
			}
		}
	}
}

// Unhinted glyphs keep their sub-pixel position: above the hinting size limit
// the outline was never grid-fit, so snapping would only cost precision.
func TestUnhintedGlyphsKeepSubPixelPosition(t *testing.T) {
	_, _, fracX, _ := glyphPlacement(10.5, 40.25, 1, text.HintingNone)
	if fracX == 0 {
		t.Error("an unhinted glyph at x=10.5 should keep its 0.5 fraction")
	}
}

// The bug that X snapping originally caused: positions accumulated from
// *rounded advances* drifted a run off its measured width — 9px across 43
// characters at 9px. Rounding each absolute position independently cannot
// accumulate, so a long run stays within half a device pixel of exact.
func TestSnappingDoesNotAccumulateAcrossARun(t *testing.T) {
	const (
		advance = 5.37 // a deliberately unfriendly fractional advance
		n       = 200
		scale   = 2.0
	)
	for i := range n {
		exact := float64(i) * advance
		got, _, _, _ := glyphPlacement(exact, 0, scale, text.HintingFull)
		if err := math.Abs(got*scale - exact*scale); err > 0.5+1e-9 {
			t.Fatalf("glyph %d: placed %.4f, exact %.4f — off by %.4f device px; "+
				"error is accumulating rather than staying bounded",
				i, got*scale, exact*scale, err)
		}
	}
}
