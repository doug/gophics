//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// What costs a render pass?
//
// design/rendering-pipeline.md measured MSAA at roughly 1.3ms per pass on a
// Mali tiler, and the reference scene at 21 passes against 1 for every other
// corpus scene — 53ms/frame. So the pass count, not the scene content, is what
// makes that frame expensive, and nothing said which construct spends them.
//
// Each case below adds one construct to the same baseline, so the difference is
// attributable. Reported, not gated: this exists to aim the work.
func TestWhatCostsARenderPass(t *testing.T) {
	base := func(inner func(c paint.Canvas)) widget.Widget {
		return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.FillRect(geom.RectXYWH(10, 10, 80, 60), paint.RGB(0.8, 0.2, 0.2))
			if inner != nil {
				inner(c)
			}
		}}
	}
	r := geom.RectXYWH(20, 20, 120, 90)

	cases := []struct {
		name string
		root widget.Widget
	}{
		{"baseline (one fill)", base(nil)},
		{"+ rect clip", base(func(c paint.Canvas) {
			c.PushClip(r)
			c.FillRect(r, paint.RGB(0.2, 0.5, 0.9))
			c.PopClip()
		})},
		{"+ rrect clip", base(func(c paint.Canvas) {
			c.PushClipRRect(r, 16)
			c.FillRect(r, paint.RGB(0.2, 0.5, 0.9))
			c.PopClip()
		})},
		{"+ nested clips", base(func(c paint.Canvas) {
			c.PushClipRRect(r, 16)
			c.PushClip(geom.RectXYWH(20, 20, 60, 90))
			c.FillRect(r, paint.RGB(0.2, 0.5, 0.9))
			c.PopClip()
			c.PopClip()
		})},
		{"+ gradient", base(func(c paint.Canvas) {
			c.FillRRectGradient(r, 8, paint.RGB(0.2, 0.8, 0.9), paint.RGB(0.9, 0.3, 0.5), true)
		})},
		{"+ sprite", base(func(c paint.Canvas) {
			c.Image(testAtlas(), geom.RectXYWH(20, 20, 32, 32))
		})},
		{"+ opacity group", widget.Opacity{Alpha: 0.5, Child: base(nil)}},
		{"+ 2 opacity groups", widget.Opacity{Alpha: 0.5,
			Child: widget.Opacity{Alpha: 0.5, Child: base(nil)}}},
	}

	for _, c := range cases {
		h, err := NewHeadless(c.root, Config{
			Size: geom.Size{W: 200, H: 150}, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		if h.RenderGPU() == nil {
			t.Skip("no GPU adapter available")
		}
		skipWithoutHardwareGPU(t)
		h.RenderGPU() // warm frame; counters describe the last frame
		e := wgpu.EncoderStats()
		t.Logf("%-22s passes=%2d draws=%3d switches=%3d", c.name, e.RenderPasses, e.DrawCalls, e.PipelineSwitches)
	}
}
