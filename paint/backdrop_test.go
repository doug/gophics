package paint

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// TestBackdropBlurSmoothsSeam drives Canvas.BackdropBlur end-to-end (paint →
// gg.Context → pixmap): a hard red|blue seam, blurred over a straddling region,
// becomes a blend there while the untouched sides stay pure.
func TestBackdropBlurSmoothsSeam(t *testing.T) {
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 40, H: 20}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 20, 20), RGB(1, 0, 0))  // left half red
	c.FillRect(geom.RectXYWH(20, 0, 20, 20), RGB(0, 0, 1)) // right half blue
	c.BackdropBlur(geom.RectXYWH(10, 0, 20, 20), 6)        // blur across the seam

	img := p.Image()
	at8 := func(x, y int) (r, g, b uint8) {
		cr, cg, cb, _ := img.At(x, y).RGBA()
		return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)
	}

	// At the seam, inside the blurred region: both channels mixed (a blend).
	if r, _, b := at8(20, 10); r < 20 || b < 20 {
		t.Errorf("seam not blended after blur: r=%d b=%d (want both non-trivial)", r, b)
	}
	// Left of the blurred region: still essentially pure red.
	if r, _, b := at8(3, 10); r < 200 || b > 30 {
		t.Errorf("red side leaked outside the blur: r=%d b=%d", r, b)
	}
	// Right of the blurred region: still essentially pure blue.
	if r, _, b := at8(37, 10); b < 200 || r > 30 {
		t.Errorf("blue side leaked outside the blur: r=%d b=%d", r, b)
	}
}
