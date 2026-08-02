//go:build gossamer_gpu && !js

package app

import (
	"image"

	"github.com/doug/gg"
	_ "github.com/doug/gg/gpu" // register gg's GPU accelerator
	"github.com/doug/gg/integration/ggcanvas"
	"github.com/doug/gogpu"
)

// gpuRenderer is a headless GPU renderer + canvas, reused across RenderGPU calls.
type gpuRenderer struct {
	r    *gogpu.Renderer
	ggc  *ggcanvas.Canvas
	w, h int
}

// RenderGPU lays out and records the scene exactly like Render, but rasterizes
// it on the GPU — offscreen render through ggcanvas plus a readback — instead of
// the CPU. It returns the physical-pixel image so tests can compare the two
// backends. Available only under -tags gossamer_gpu; returns nil when no GPU
// adapter is present (so callers can t.Skip).
func (h *Headless) RenderGPU() image.Image {
	h.Core.drainPosted()
	h.Core.Layout(h.size)
	h.Core.RecordScene(h.size, h.scale) // record; the GPU replays the full scene

	pw, ph := int(h.size.W*h.scale), int(h.size.H*h.scale)
	if pw <= 0 || ph <= 0 {
		return nil
	}

	g, _ := h.gpu.(*gpuRenderer)
	if g == nil {
		r, err := gogpu.NewHeadlessRenderer()
		if err != nil {
			return nil
		}
		g = &gpuRenderer{r: r}
		h.gpu = g
	}
	if g.ggc == nil || g.w != pw || g.h != ph {
		ggc, err := ggcanvas.New(g.r.GPUContextProvider(), pw, ph)
		if err != nil {
			return nil
		}
		g.ggc, g.w, g.h = ggc, pw, ph
	}

	img, err := g.r.RenderToImage(pw, ph, func(dc *gogpu.Context) {
		_ = g.ggc.Draw(func(cc *gg.Context) {
			h.Core.ReplayScene(h.Core.Painter.GPUCanvas(cc))
		})
		_ = g.ggc.Render(dc.RenderTarget())
	})
	if err != nil {
		return nil
	}
	return img
}
