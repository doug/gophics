//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Letter spacing must not depend on where a word starts.
//
// This is the failure mode snapXGrid existed to prevent: if each glyph's
// position is rounded independently, adjacent advances jitter by ±1px and gaps
// open inside words — "anyway" renders as "an yway". Removing snapXGrid (F7)
// gave that guarantee up at the quad level, where positions are now an integer
// bucket plus a sub-pixel mask variant, so the old test's proxy — differences
// between quad X0 values — measures the bucket and reads as jitter even when
// nothing visible moves.
//
// The property is about what is drawn, so it is asserted on what is drawn. The
// CPU rasterizer places glyphs at exact shaper positions by construction, so if
// the GPU agrees with it at every sub-pixel start offset, spacing cannot be
// jittering: a backend that opened a gap would diverge at the offsets where its
// rounding crossed a pixel boundary and not at the others.
func TestGPUTextSpacingIsIndependentOfSubpixelStart(t *testing.T) {
	for _, base := range []float32{100, 100.25, 100.5, 100.75} {
		root := widget.Canvas{Draw: func(c paint.Canvas, _ geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.TextIn("", "anyway anyway", geom.Pt{X: base, Y: 40}, 13, paint.RGB(0, 0, 0))
		}}
		h, err := NewHeadless(root, Config{
			Size: geom.Size{W: 320, H: 70}, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		cpu := toRGBA(h.Render())
		gpuImg := h.RenderGPU()
		if gpuImg == nil {
			t.Skip("no GPU adapter available")
		}
		skipWithoutHardwareGPU(t)
		gpu := toRGBA(gpuImg)

		var diff, total int
		b := cpu.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				o := cpu.PixOffset(x, y)
				d := 0
				for k := range 4 {
					dd := int(cpu.Pix[o+k]) - int(gpu.Pix[o+k])
					if dd < 0 {
						dd = -dd
					}
					d = max(d, dd)
				}
				if d > 32 {
					diff++
				}
				total++
			}
		}
		frac := float64(diff) / float64(total)
		t.Logf("base x=%.2f: %d/%d pixels differ = %.3f%%", base, diff, total, frac*100)
		// A gap opening inside a word is a whole glyph displaced, which is far
		// past edge-AA noise on a mostly-white canvas.
		if frac > 0.03 {
			t.Errorf("base x=%.2f: GPU and CPU disagree on %.2f%% — spacing may be "+
				"jittering with the sub-pixel start", base, frac*100)
		}
	}
}
