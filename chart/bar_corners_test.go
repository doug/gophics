package chart

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// A bar meets its axis square. Rounding all four corners lifts the foot off the
// baseline and shows a sliver of background under it, so the bar reads as
// floating rather than as measured from an axis.
func TestBarFootIsSquareAtTheBaseline(t *testing.T) {
	const w, h = 40, 60
	for _, tc := range []struct {
		name         string
		baseAtBottom bool
		probeY       int // the row just inside the baseline end
		roundedY     int // the row just inside the value end
	}{
		{"bar grows up from the axis", true, h - 1, 0},
		{"bar hangs down from the axis", false, 0, h - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := paint.NewPainter()
			c := p.BeginOffscreen(geom.Size{W: w, H: h}, 1)
			c.Clear(paint.Color{R: 1, G: 1, B: 1, A: 1})
			fillBarEnd(c, geom.RectXYWH(0, 0, w, h), 8, tc.baseAtBottom,
				paint.Color{R: 0, G: 0, B: 0, A: 1})
			img := p.SurfaceRGBA()

			filled := func(x, y int) bool {
				i := img.PixOffset(x, y)
				return img.Pix[i] < 128 // dark = bar
			}
			// The two corners at the baseline end must be filled...
			if !filled(0, tc.probeY) || !filled(w-1, tc.probeY) {
				t.Errorf("the baseline corners are not filled — the bar is not sitting on its axis")
			}
			// ...and the two at the value end must not be, or nothing was rounded.
			if filled(0, tc.roundedY) && filled(w-1, tc.roundedY) {
				t.Errorf("the value-end corners are square — the bar lost its rounding entirely")
			}
		})
	}
}

// A bar shorter than its corner radius has no room to round; it must still
// draw, and draw square, rather than collapsing or over-rounding.
func TestVeryShortBarStillDraws(t *testing.T) {
	p := paint.NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 20, H: 20}, 1)
	c.Clear(paint.Color{R: 1, G: 1, B: 1, A: 1})
	fillBarEnd(c, geom.RectXYWH(0, 18, 20, 2), 8, true, paint.Color{A: 1})
	img := p.SurfaceRGBA()
	if img.Pix[img.PixOffset(10, 19)] >= 128 {
		t.Error("a 2px bar did not draw")
	}
}
