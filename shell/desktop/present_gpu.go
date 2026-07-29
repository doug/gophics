//go:build gossamer_gpu

// Package desktop's GPU build (‑tags gossamer_gpu) rasterizes each frame on
// the GPU via gg's accelerator instead of the CPU: gossamer records its scene,
// replays it onto a GPU‑backed gg.Context (ggcanvas), and composites directly
// to the swapchain. This removes the CPU‑rasterizer throughput ceiling that
// makes full‑surface animation (scroll, transitions) jittery (PLAN.md M5).
package desktop

import (
	"log"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // registers gg's GPU accelerator (SDF/tiled raster)
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"

	"github.com/doug/gossamer/shell"
)

// onFrameStart lazily creates the GPU canvas once the surface exists (the
// device provider is only available after the window opens), and keeps it
// sized to the surface.
func (w *window) onFrameStart(dc *gogpu.Context) {
	lw, lh := dc.Width(), dc.Height()
	if lw <= 0 || lh <= 0 {
		return
	}
	if c, ok := w.ggc.(*ggcanvas.Canvas); ok {
		_ = c.Resize(lw, lh)
		return
	}
	provider := w.app.GPUContextProvider()
	if provider == nil {
		return
	}
	c, err := ggcanvas.New(provider, lw, lh)
	if err != nil {
		log.Printf("gossamer/desktop: ggcanvas init: %v", err)
		return
	}
	w.ggc = c
}

// gpuTarget carries the GPU canvas and swapchain target for one frame; the app
// layer type-asserts it (app.gpuCanvasTarget) and drives the scene replay.
type gpuTarget struct {
	ggc *ggcanvas.Canvas
	dc  *gogpu.Context
}

func (f *frame) Target() shell.Target {
	c, ok := f.w.ggc.(*ggcanvas.Canvas)
	if !ok {
		return nil // GPU canvas not ready yet; app falls back to CPU present
	}
	return gpuTarget{ggc: c, dc: f.dc}
}

// RenderGPU runs the scene replay against the GPU-backed gg.Context, then
// composites the result to the swapchain.
func (t gpuTarget) RenderGPU(replay func(*gg.Context)) {
	_ = t.ggc.Draw(func(cc *gg.Context) { replay(cc) })
	if err := t.ggc.Render(t.dc.RenderTarget()); err != nil {
		log.Printf("gossamer/desktop: gpu render: %v", err)
	}
}
