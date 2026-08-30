//go:build gophics_gpu

package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// inkExtentX returns the first and last column containing ink.
func inkExtentX(im *image.RGBA) (lo, hi int) {
	b := im.Bounds()
	lo, hi = -1, -1
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			o := im.PixOffset(x, y)
			if int(im.Pix[o])+int(im.Pix[o+1])+int(im.Pix[o+2]) < 3*128 {
				if lo < 0 {
					lo = x
				}
				hi = x
				break
			}
		}
	}
	return lo, hi
}

// Does what the GPU draws match what the layout engine measured?
//
// MeasureWidthIn sums the shaper's unrounded advances, and every layout
// decision comes from it: where a line breaks, where centred text starts, where
// a caret sits. The CPU rasterizer draws from those same unrounded positions,
// so measurement and CPU rendering agree by construction.
//
// The GPU does not. glyph_mask_engine.snapXGrid replaces each glyph's position
// with an accumulation of *rounded* advances — a deliberate hinted-text
// technique that keeps vertical stems on pixel boundaries, and the right call
// for crispness taken on its own. Applied to one backend only, it makes the
// drawn string a different width from the measured one, and the error
// accumulates with every glyph instead of cancelling.
//
// Reported, not gated: which backend should change is a design decision, and
// all three answers move text appearance framework-wide. See
// design/rendering-pipeline.md.
func TestGPUTextWidthVersusMeasuredWidth(t *testing.T) {
	const s = "The quick brown fox jumps over the lazy dog"
	for _, size := range []float32{9, 11, 13, 15, 20} {
		root := widget.Canvas{Draw: func(c paint.Canvas, _ geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.TextIn("", s, geom.Pt{X: 10, Y: 40}, size, paint.RGB(0, 0, 0))
		}}
		h, err := NewHeadless(root, Config{
			Size: geom.Size{W: 520, H: 80}, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		measured := h.core.Painter.MeasureWidthIn("", s, size)

		cpuLo, cpuHi := inkExtentX(toRGBA(h.Render()))
		gpuImg := h.RenderGPU()
		if gpuImg == nil {
			t.Skip("no GPU adapter available")
		}
		gpuLo, gpuHi := inkExtentX(toRGBA(gpuImg))

		cpuW, gpuW := float32(cpuHi-cpuLo), float32(gpuHi-gpuLo)
		t.Logf("size %4.1f  measured %7.2f | cpu ink %6.1f (%+5.1f) | gpu ink %6.1f (%+5.1f) | gpu-cpu %+5.1f",
			size, measured, cpuW, cpuW-measured, gpuW, gpuW-measured, gpuW-cpuW)
	}
}
