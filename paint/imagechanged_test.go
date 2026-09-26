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

// sliceImg is a value-typed image whose dynamic type is not comparable: a
// map keyed by it panics.
type sliceImg struct{ pix []uint8 }

func (sliceImg) ColorModel() color.Model { return color.RGBAModel }
func (sliceImg) Bounds() image.Rectangle { return image.Rect(0, 0, 4, 4) }
func (sliceImg) At(x, y int) color.Color { return color.RGBA{R: 255, A: 255} }

// Canvas.Image promises that a struct-typed image draws without a panic; the
// texture cache is a map keyed by the image value, and that promise has to
// hold there too — and for DrawSprite and ImageChanged, which use the same
// caches.
func TestNonComparableImageDrawsWithoutPanic(t *testing.T) {
	p := NewPainter()
	img := sliceImg{pix: []uint8{1}}
	c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	c.Image(img, geom.RectXYWH(0, 0, 4, 4))
	c.DrawSprite(img, Sprite{Src: image.Rect(0, 0, 4, 4), Dst: geom.RectXYWH(4, 4, 4, 4)})
	c.DrawSprite(img, Sprite{Src: image.Rect(0, 0, 4, 4), Dst: geom.RectXYWH(4, 4, 4, 4), Tint: Color{1, 1, 1, 1}})
	p.ImageChanged(img)
	if r, _, _, _ := p.Image().At(2, 2).RGBA(); r>>8 < 200 {
		t.Errorf("the image did not draw (red = %d)", r>>8)
	}
}

// A tinted sprite is cached by a 4-bit quantized tint, so its pixels must be
// rasterized with that same quantized tint: otherwise two sprites whose tints
// share a key share the first caller's exact colour, and which one wins
// depends on draw order.
func TestTintedSpriteRendersTheQuantizedTint(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fill(atlas, color.RGBA{255, 255, 255, 255})
	red := func(p *Painter, tint float32) uint8 {
		c := p.BeginOffscreen(geom.Size{W: 4, H: 4}, 1)
		c.DrawSprite(atlas, Sprite{Src: image.Rect(0, 0, 4, 4), Dst: geom.RectXYWH(0, 0, 4, 4),
			Tint: Color{R: tint, G: 1, B: 1, A: 1}})
		r, _, _, _ := p.Image().At(2, 2).RGBA()
		return uint8(r >> 8)
	}
	// 0.53 and 0.50 both quantize to 7/15.
	want := mul8(255, 7.0/15)
	p := NewPainter()
	if got := red(p, 0.53); got != want {
		t.Errorf("first draw at tint 0.53: red = %d, want %d (the quantized tint)", got, want)
	}
	if got := red(p, 0.50); got != want {
		t.Errorf("second draw at tint 0.50: red = %d, want %d", got, want)
	}
	// And drawn the other way round, the same answer.
	p = NewPainter()
	if got := red(p, 0.50); got != want {
		t.Errorf("first draw at tint 0.50: red = %d, want %d", got, want)
	}
	if got := red(p, 0.53); got != want {
		t.Errorf("second draw at tint 0.53: red = %d, want %d", got, want)
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

// A tinted sprite is cached separately from the plain image, keyed by the
// same atlas, so ImageChanged has to drop those copies too — or a tinted
// sprite keeps drawing the pixels the atlas held before the rewrite.
func TestImageChangedRefreshesTintedSprites(t *testing.T) {
	p := NewPainter()
	atlas := whiteAtlas(4, 4)
	tint := Sprite{Tint: Color{R: 1, G: 1, B: 1, A: 1}}
	if px := spriteCentre(p, atlas, tint); px.R < 200 || px.G < 200 {
		t.Fatalf("first draw = %v, want white; the rest of this test would prove nothing", px)
	}
	fill(atlas, color.RGBA{R: 255, A: 255})
	if px := spriteCentre(p, atlas, tint); px.G < 200 {
		t.Skip("tinted sprites are no longer cached; this test is obsolete")
	}
	p.ImageChanged(atlas)
	if px := spriteCentre(p, atlas, tint); px.R < 200 || px.G > 50 {
		t.Errorf("after ImageChanged the tinted sprite still drew the old pixels (%v, want red)", px)
	}
}

// wrapImg is a struct-typed image whose *type* is comparable (an interface
// field) but whose *value* need not be: the field can hold a slice-backed
// image, and a map keyed by that value panics just as sliceImg does.
type wrapImg struct{ inner image.Image }

func (w wrapImg) ColorModel() color.Model { return w.inner.ColorModel() }
func (w wrapImg) Bounds() image.Rectangle { return w.inner.Bounds() }
func (w wrapImg) At(x, y int) color.Color { return w.inner.At(x, y) }

// The comparability check has to look at the dynamic value, not the static
// type: reflect.Type.Comparable says yes to wrapImg and the map lookup then
// panics on the sliceImg inside it.
func TestComparableTypeWithUnhashableValueDrawsWithoutPanic(t *testing.T) {
	p := NewPainter()
	img := wrapImg{inner: sliceImg{pix: []uint8{1}}}
	c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	c.Image(img, geom.RectXYWH(0, 0, 4, 4))
	c.DrawSprite(img, Sprite{Src: image.Rect(0, 0, 4, 4), Dst: geom.RectXYWH(4, 4, 4, 4)})
	c.DrawSprite(img, Sprite{Src: image.Rect(0, 0, 4, 4), Dst: geom.RectXYWH(4, 4, 4, 4), Tint: Color{1, 1, 1, 1}})
	p.ImageChanged(img)
	if r, _, _, _ := p.Image().At(2, 2).RGBA(); r>>8 < 200 {
		t.Errorf("the image did not draw (red = %d)", r>>8)
	}
	// And the same wrapper around a pointer image is cached as usual.
	ptr := wrapImg{inner: whiteAtlas(4, 4)}
	c.Image(ptr, geom.RectXYWH(0, 0, 4, 4))
	if _, ok := p.imgBufs[ptr]; !ok {
		t.Error("a comparable wrapper value was not cached")
	}
}
