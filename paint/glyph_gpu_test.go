//go:build gossamer_gpu

package paint

import (
	"image"
	"testing"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // register the GPU accelerator (as the gossamer_gpu build does)
)

func countOpaque(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				n++
			}
		}
	}
	return n
}

// TestGlyphScratchNotBlankWithAccelerator guards the fix in Painter.runFor:
// with a GPU accelerator registered, a default (auto) gg.Context defers its
// fill to the GPU, so Image() reads back blank — which would make every cached
// text run render as nothing. Forcing RasterizerAnalytic keeps glyph
// rasterization on the CPU so the bitmap is real (then blitted as a texture).
func TestGlyphScratchNotBlankWithAccelerator(t *testing.T) {
	auto := gg.NewContext(40, 40)
	auto.SetRGB(1, 1, 1)
	auto.DrawRectangle(5, 5, 30, 30)
	_ = auto.Fill()
	if countOpaque(auto.Image()) != 0 {
		t.Skip("auto context not blank — GPU accelerator not deferring here; fix is moot")
	}
	an := gg.NewContext(40, 40)
	an.SetRasterizerMode(gg.RasterizerAnalytic)
	an.SetRGB(1, 1, 1)
	an.DrawRectangle(5, 5, 30, 30)
	_ = an.Fill()
	if countOpaque(an.Image()) == 0 {
		t.Fatal("RasterizerAnalytic produced a blank image under a registered accelerator; glyphs would vanish on the GPU backend")
	}
}
