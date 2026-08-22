package gg

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/internal/clip"
)

// The per-pixel clip closure is skipped only for a rectangle on whole pixels,
// where the renderer's scanline bounds describe it exactly. A fractional
// rectangle must keep it: SetClipBounds truncates the near edge, so it admits
// up to a pixel the clip should exclude, and the closure is what trims that.
func TestWholePixelRectDetection(t *testing.T) {
	whole := []clip.Rect{
		{X: 0, Y: 0, W: 100, H: 50},
		{X: 12, Y: 40, W: 8, H: 8},
		{X: -30, Y: -10, W: 60, H: 20},
	}
	for _, r := range whole {
		if !isWholePixelRect(r) {
			t.Errorf("%+v is on whole pixels but was not recognised; the closure "+
				"would keep costing ~8ns a pixel to re-prove the scanline bounds", r)
		}
	}

	fractional := []clip.Rect{
		{X: 10.5, Y: 0, W: 100, H: 50},
		{X: 0, Y: 0.25, W: 100, H: 50},
		{X: 0, Y: 0, W: 99.5, H: 50},
		{X: 0, Y: 0, W: 100, H: 49.75},
	}
	for _, r := range fractional {
		if isWholePixelRect(r) {
			t.Errorf("%+v has a fractional edge but was treated as whole-pixel; "+
				"dropping the closure there lets up to a pixel through the clip", r)
		}
	}
}
