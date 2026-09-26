//go:build gophics_gpu

package paint

import (
	"image/color"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
)

// A tinted sprite with Tint.A < 1 must composite the same on the GPU as on the
// CPU. The tinted image is premultiplied; the CPU blit clamps colour to alpha
// and forgives an image that is not, while the GPU uploads the bytes verbatim
// and blends source-over — so an invalid image renders opaque there and
// nowhere else. Skips (a capability guard, not a defect) without a headless
// adapter.
func TestGPUTintedSpriteAlphaMatchesCPU(t *testing.T) {
	atlas := whiteAtlas(4, 4)
	draw := func(c Canvas) {
		c.Clear(RGB(0, 0, 0))
		c.DrawSprite(atlas, Sprite{Src: atlas.Bounds(), Dst: geom.RectXYWH(0, 0, 16, 16), Alpha: 0.5})
		c.DrawSprite(atlas, Sprite{Src: atlas.Bounds(), Dst: geom.RectXYWH(16, 0, 16, 16), Tint: Color{R: 1, G: 1, B: 1, A: 0.5}})
		c.DrawSprite(atlas, Sprite{Src: atlas.Bounds(), Dst: geom.RectXYWH(32, 0, 16, 16), Tint: Color{R: 1, A: 0.5}})
	}
	pc := NewPainter()
	draw(pc.BeginOffscreen(geom.Size{W: 48, H: 16}, 1))
	cpu := pc.SurfaceRGBA()

	pg := NewPainter()
	pg.BeginOffscreen(geom.Size{W: 48, H: 16}, 1)
	gpu := renderGPU(t, 48, 16, func(cc *gg.Context) { draw(pg.GPUCanvas(cc)) })

	for _, s := range []struct {
		name string
		x    int
	}{{"Alpha 0.5", 8}, {"Tint.A 0.5", 24}, {"Tint red 0.5", 40}} {
		c, g := cpu.RGBAAt(s.x, 8), gpu.RGBAAt(s.x, 8)
		if !closeRGBA(c, g, 8) {
			t.Errorf("%s: CPU %v, GPU %v", s.name, c, g)
		}
	}
	// And Tint.A really is half-transparent, not a no-op.
	if c := gpu.RGBAAt(24, 8); c.R > 140 {
		t.Errorf("GPU Tint.A=0.5 over black drew R=%d; should be about 128", c.R)
	}
}

func closeRGBA(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol && d(a.A, b.A) <= tol
}
