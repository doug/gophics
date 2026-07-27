//go:build gossamer_gpu

package app

import (
	"log"

	"github.com/gogpu/gg"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// gpuCanvasTarget is a frame Target that rasterizes on the GPU: RenderGPU runs
// the scene replay against a GPU-backed gg.Context, then composites the result
// to the surface. The desktop shell's GPU build returns one.
type gpuCanvasTarget interface {
	RenderGPU(replay func(*gg.Context))
}

// present rasterizes on the GPU when the frame offers a GPU target (M5),
// otherwise falls back to the CPU rasterizer. Built only with -tags
// gossamer_gpu; the default build uses present_cpu.go.
func (h *shellHandler) present(f shell.Frame, changed bool, damage geom.Rect) {
	if gt, ok := f.Target().(gpuCanvasTarget); ok {
		gt.RenderGPU(func(cc *gg.Context) {
			// GPU rasterizes the whole frame each time; replay in full.
			h.core.ReplayScene(h.core.Painter.GPUCanvas(cc))
		})
		return
	}
	if changed {
		canvas := h.core.Painter.Begin(f)
		h.core.ReplayDamaged(canvas, damage)
	}
	if err := h.core.Painter.End(f); err != nil {
		log.Printf("gossamer: present: %v", err)
	}
}
