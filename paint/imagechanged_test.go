package paint

import (
	"image"
	"image/color"
	"testing"

	"github.com/doug/gophics/geom"
)

// fill paints every pixel of an image one colour.
func fill(img *image.RGBA, c color.RGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
}

// drawOnce draws img into a fresh surface and returns the pixel at its centre.
func drawOnce(p *Painter, img *image.RGBA) (r, g, b uint8) {
	c := p.BeginOffscreen(geom.Size{W: 20, H: 20}, 1)
	c.Image(img, geom.RectXYWH(0, 0, 20, 20))
	out := p.Image()
	cr, cg, cb, _ := out.At(10, 10).RGBA()
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)
}

// Drawing copies an image's pixels and keeps the copy, keyed by the image
// value. That is right for an icon and wrong for anything that changes, and
// the failure is silent: the image keeps drawing whatever it held the first
// time.
//
// This is not hypothetical. The camera preview froze on its first frames for
// exactly this reason, and it took a screenshot diff to see it, because every
// layer above — frames arriving, the warp running, the canvas re-recording —
// was working and reported healthy.
func TestImageChangedRefreshesTheCachedPixels(t *testing.T) {
	p := NewPainter()
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))

	fill(img, color.RGBA{R: 255, A: 255})
	if r, _, _ := drawOnce(p, img); r < 200 {
		t.Fatalf("the first draw did not show the image (red = %d); the rest of "+
			"this test would prove nothing", r)
	}

	// Rewrite the same image and draw it again, without saying anything.
	fill(img, color.RGBA{B: 255, A: 255})
	if _, _, b := drawOnce(p, img); b > 100 {
		t.Skip("the painter no longer caches image pixels; this test is obsolete")
	}

	// Now say so.
	fill(img, color.RGBA{B: 255, A: 255})
	p.ImageChanged(img)
	r, _, b := drawOnce(p, img)
	if b < 200 || r > 100 {
		t.Errorf("after ImageChanged the draw still showed the old pixels "+
			"(r=%d b=%d, want blue)", r, b)
	}
}

// Rotating a pool does not avoid the cache, which is the trap worth pinning:
// every buffer is cached in turn, and from then on the display cycles stale
// snapshots. A camera preview built that way shows its first frames and stops.
func TestRotatingBuffersStillNeedImageChanged(t *testing.T) {
	p := NewPainter()
	pool := [2]*image.RGBA{
		image.NewRGBA(image.Rect(0, 0, 20, 20)),
		image.NewRGBA(image.Rect(0, 0, 20, 20)),
	}

	// Two frames of red fill both slots and both get cached.
	for i := range pool {
		fill(pool[i], color.RGBA{R: 255, A: 255})
		drawOnce(p, pool[i])
	}

	// The third frame reuses slot 0 with new pixels — the case a pool is
	// supposed to solve and does not.
	fill(pool[0], color.RGBA{G: 255, A: 255})
	if _, g, _ := drawOnce(p, pool[0]); g > 100 {
		t.Skip("the painter no longer caches image pixels; this test is obsolete")
	}

	p.ImageChanged(pool[0])
	if _, g, _ := drawOnce(p, pool[0]); g < 200 {
		t.Errorf("a pooled buffer stayed stale after ImageChanged (green = %d)", g)
	}
}

// A nil image and a nil painter are both no-ops, so a caller need not guard.
func TestImageChangedIsNilSafe(t *testing.T) {
	var p *Painter
	p.ImageChanged(nil)
	NewPainter().ImageChanged(nil)
}
