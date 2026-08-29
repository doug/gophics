package paint

import (
	"image"
	"image/color"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
)

// Sprite describes one blit of a source region of an atlas image into a
// destination rectangle. Passing the same atlas image.Image across many
// DrawSprite calls shares a single cached texture — the reason to use an atlas
// rather than one image per tile (which hits the texture-cache budget).
type Sprite struct {
	Src      image.Rectangle // source region in the atlas (pixels)
	Dst      geom.Rect       // destination (logical pixels)
	Alpha    float32         // 0 → 1 (opaque)
	Tint     Color           // multiplies the sprite's RGBA; zero alpha → no tint
	Rotation float32         // radians, clockwise about Dst's center
	FlipX    bool            // mirror horizontally about Dst's center
	Nearest  bool            // nearest-neighbor sampling (crisp pixel art)
}

// tintKey identifies a cached tinted sub-image: the source atlas, its region,
// and the tint quantized to 4 bits per channel (bounds the cache).
type tintKey struct {
	atlas      image.Image
	src        image.Rectangle
	r, g, b, a uint8
}

func q4(f float32) uint8 {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return uint8(f * 15)
}

func mul8(v uint8, f float32) uint8 {
	n := float32(v) * f
	if n > 255 {
		n = 255
	}
	if n < 0 {
		n = 0
	}
	return uint8(n)
}

// tinted returns a cached texture of src (from atlas) with every pixel
// multiplied by tint — used for lighting and palette-swaps.
func (p *Painter) tinted(atlas image.Image, src image.Rectangle, tint Color) *gg.ImageBuf {
	key := tintKey{atlas, src, q4(tint.R), q4(tint.G), q4(tint.B), q4(tint.A)}
	if b, ok := p.tintBufs[key]; ok {
		return b
	}
	if len(p.tintBufs) > 512 {
		evictHalf(p.tintBufs)
	}
	w, h := src.Dx(), src.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			r, g, b, a := atlas.At(src.Min.X+x, src.Min.Y+y).RGBA() // 16-bit, straight
			out.SetRGBA(x, y, color.RGBA{
				R: mul8(uint8(r>>8), tint.R),
				G: mul8(uint8(g>>8), tint.G),
				B: mul8(uint8(b>>8), tint.B),
				A: mul8(uint8(a>>8), tint.A),
			})
		}
	}
	buf := gg.ImageBufFromImage(out)
	p.tintBufs[key] = buf
	return buf
}
