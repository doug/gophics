package paint

import (
	"image"
	"image/color"
	"testing"

	"github.com/doug/gophics/geom"
)

// whiteAtlas returns an opaque white w×h atlas.
func whiteAtlas(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(img, color.RGBA{255, 255, 255, 255})
	return img
}

// spriteCentre draws one sprite over black and returns its centre pixel.
func spriteCentre(p *Painter, atlas image.Image, s Sprite) color.RGBA {
	c := p.BeginOffscreen(geom.Size{W: 16, H: 16}, 1)
	c.Clear(RGB(0, 0, 0))
	s.Src = atlas.Bounds()
	s.Dst = geom.RectXYWH(0, 0, 16, 16)
	c.DrawSprite(atlas, s)
	return p.SurfaceRGBA().RGBAAt(8, 8)
}

// A tint with alpha below one must produce a valid premultiplied image: every
// channel scaled by the tint alpha, never RGB above A. The CPU blit clamps an
// invalid one and hides the mistake; the GPU uploads the bytes as they are and
// composites source-over, so a half-transparent tinted sprite rendered fully
// opaque there. The cached buffer is what both backends draw, so it is what
// this test inspects.
func TestTintedSpriteStaysPremultiplied(t *testing.T) {
	p := NewPainter()
	atlas := whiteAtlas(4, 4)
	px := tintPixels(atlas, atlas.Bounds(), tintKey{a: q4(0.5), r: 15, g: 15, b: 15}).RGBAAt(2, 2)
	if px.R > px.A || px.G > px.A || px.B > px.A {
		t.Fatalf("tinted pixel %v has colour above alpha: not premultiplied", px)
	}
	if want := mul8(255, unq4(q4(0.5))); px.A != want {
		t.Errorf("tinted alpha = %d, want %d (the quantized tint alpha)", px.A, want)
	}

	// Drawn over black, Tint.A = 0.5 and Alpha = 0.5 are the same thing.
	tinted := spriteCentre(p, atlas, Sprite{Tint: Color{R: 1, G: 1, B: 1, A: 0.5}})
	alpha := spriteCentre(p, atlas, Sprite{Alpha: 0.5})
	if d := int(tinted.R) - int(alpha.R); d < -8 || d > 8 {
		t.Errorf("Tint.A=0.5 drew R=%d, Alpha=0.5 drew R=%d; they should match", tinted.R, alpha.R)
	}
}

// A tint's colour channels are relative to the tint alpha: Tint{1,0,0,0.5}
// is half-transparent red, not quarter-strength red.
func TestTintedSpriteColourIsScaledByTintAlpha(t *testing.T) {
	atlas := whiteAtlas(4, 4)
	px := tintPixels(atlas, atlas.Bounds(), tintKey{a: q4(0.5), r: 15}).RGBAAt(1, 1)
	if px.R != px.A || px.G != 0 || px.B != 0 {
		t.Errorf("Tint{1,0,0,0.5} on white = %v, want R == A and G, B zero", px)
	}
}
