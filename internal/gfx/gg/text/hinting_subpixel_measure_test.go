package text

import (
	"fmt"
	"testing"
)

// Documents why hinted glyphs must be placed on whole device pixels.
//
// The GPU mask cache quantizes a glyph's sub-pixel X fraction to quarters, so
// off-grid placement means drawing from one of four masks whose stems fall
// differently against the pixel grid. "Saturation" here is the share of inked
// pixels that are nearly fully covered: a crisp stem saturates, a stem split
// across two pixels does not.
//
// The absolute figures matter less than the swing between offsets, because
// adjacent glyphs in a run draw different offsets — that swing, inside one
// word, is text that looks blurry in places and too heavy in others.
//
// This is a guard, not just a measurement. If the swing at small sizes ever
// becomes negligible (a finer quantization, a different rasterizer), then
// glyphPlacement's X snapping is buying nothing and its cost — up to a pixel
// of variation in inter-glyph gaps — should be reconsidered.
func TestHintingSubpixelStability(t *testing.T) {
	parsed := parseWithOwn(t, loadTestFontData(t, "goregular.ttf"))
	rast := NewGlyphMaskRasterizer()

	worst := 0.0
	for _, size := range []float64{9, 11, 13, 14, 18, 22, 28, 36} {
		lo, hi := 1.0, 0.0
		var report []string
		for _, frac := range []float64{0, 0.25, 0.5, 0.75} {
			var ink, sat int
			for _, r := range "nlimuhbdE" { // stem-heavy letters
				gid := parsed.GlyphIndex(r)
				if gid == 0 {
					continue
				}
				res, err := rast.RasterizeHinted(parsed, GlyphID(gid), size, frac, 0, HintingFull)
				if err != nil || res == nil {
					continue
				}
				for _, v := range res.Mask {
					if v > 8 {
						ink++
					}
					if v > 230 {
						sat++
					}
				}
			}
			if ink == 0 {
				t.Skip("no glyphs rasterized from this font")
			}
			s := float64(sat) / float64(ink)
			lo, hi = min(lo, s), max(hi, s)
			report = append(report, fmt.Sprintf("%.2f→%.0f%%", frac, s*100))
		}
		swing := (hi - lo) * 100
		if size <= 14 {
			worst = max(worst, swing)
		}
		t.Logf("size %4.0fpx  saturation by sub-pixel offset: %v  swing=%.1f pts", size, report, swing)
	}

	if worst < 10 {
		t.Errorf("largest small-size swing is only %.1f points — sub-pixel offsets "+
			"no longer change how a glyph is inked, so glyphPlacement's X snapping "+
			"may be costing inter-glyph spacing for nothing", worst)
	}
}
