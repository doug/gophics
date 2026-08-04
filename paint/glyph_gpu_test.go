//go:build gophics_gpu

package paint

import (
	"image"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	_ "github.com/doug/gophics/internal/gfx/gg/gpu" // register the GPU accelerator (as the gophics_gpu build does)
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

// TestGPUDisabledContextRastersOnCPU guards the fix that gophics's CPU
// contexts (the Painter surface and the glyph atlas) rely on: with a GPU
// accelerator registered process-globally, a default context defers its fill
// to the GPU and reads back blank — which made cached text runs and images
// render as nothing under the GPU build. SetGPUDisabled(true) keeps the
// context fully on the CPU (fills and image draws).
func TestGPUDisabledContextRastersOnCPU(t *testing.T) {
	// A default (auto) context defers to the GPU → blank readback headless.
	auto := gg.NewContext(40, 40)
	auto.SetRGB(1, 1, 1)
	auto.DrawRectangle(5, 5, 30, 30)
	_ = auto.Fill()
	if countOpaque(auto.Image()) != 0 {
		t.Skip("auto context not blank — accelerator not deferring here; fix is moot")
	}

	// SetGPUDisabled keeps fills on the CPU.
	fill := gg.NewContext(40, 40)
	fill.SetGPUDisabled(true)
	fill.SetRGB(1, 1, 1)
	fill.DrawRectangle(5, 5, 30, 30)
	_ = fill.Fill()
	if countOpaque(fill.Image()) == 0 {
		t.Fatal("SetGPUDisabled fill produced a blank image under a registered accelerator")
	}

	// SetGPUDisabled keeps image blits on the CPU too (was the NetworkImage bug).
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	buf := gg.ImageBufFromImage(src)
	blit := gg.NewContext(40, 40)
	blit.SetGPUDisabled(true)
	blit.DrawImageEx(buf, gg.DrawImageOptions{X: 5, Y: 5, DstWidth: 30, DstHeight: 30})
	if countOpaque(blit.Image()) == 0 {
		t.Fatal("SetGPUDisabled image blit produced a blank image under a registered accelerator")
	}
}
