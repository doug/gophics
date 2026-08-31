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
// MSAA costs roughly 1.3ms per render pass on a Mali tiler, and the reference
// scene once ran 21 passes against 1 for every other corpus scene — 53ms/frame.
// So the pass count, not the scene content, is what made that frame expensive,
// and nothing said which construct spends them.
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
		// One draw inside the group: folded into that draw, so no pass. This is
		// the shape most opacity in a UI has — a fade over a single thing.
		{"+ 1-draw opacity (folded)", widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.PushOpacity(0.5)
			c.FillRRect(r, 8, paint.RGB(0.2, 0.5, 0.9))
			c.PopOpacity()
		}}},
		// Two draws: the group alpha applies to their composite, so folding it
		// per-draw would double-blend the overlap. Keeps its pass.
		{"+ 2-draw opacity (kept)", widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.PushOpacity(0.5)
			c.FillRRect(r, 8, paint.RGB(0.2, 0.5, 0.9))
			c.FillRRect(geom.RectXYWH(60, 40, 90, 70), 8, paint.RGB(0.9, 0.4, 0.2))
			c.PopOpacity()
		}}},
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
		t.Logf("%-26s passes=%2d draws=%3d switches=%3d", c.name, e.RenderPasses, e.DrawCalls, e.PipelineSwitches)
		// The two opacity cases are gated, because the fold is the whole reason
		// the reference scene went from 21 passes to 1 and 53ms to 12ms on a
		// Mali tiler. Losing it silently would put both back.
		switch c.name {
		case "+ 1-draw opacity (folded)":
			if e.RenderPasses != 1 {
				t.Errorf("a single-draw opacity group cost %d passes, want 1: the fold "+
					"in GPURenderContext.PopLayer is not firing", e.RenderPasses)
			}
		case "+ 2-draw opacity (kept)":
			if e.RenderPasses != 2 {
				t.Errorf("a two-draw opacity group cost %d passes, want 2: folding it "+
					"would double-blend where the two draws overlap", e.RenderPasses)
			}
		}
	}
}
