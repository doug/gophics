package app

import (
	"log"

	"github.com/doug/gophics/internal/gfx/gg"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// wireMedia publishes the window's optional media-capture capabilities (camera,
// audio) to the widget tree, or leaves them nil when the platform lacks them.
func (h *shellHandler) wireMedia(w shell.Window) {
	if mw, ok := w.(shell.MediaWindow); ok {
		h.core.Owner.Camera = mw.Camera()
		h.core.Owner.Audio = mw.Audio()
	}
	if hw, ok := w.(shell.HapticWindow); ok {
		h.core.Owner.Haptic = hw.Haptic()
	}
}

// gpuCanvasTarget is a frame Target that rasterizes on the GPU: RenderGPU runs
// the scene replay against a GPU-backed gg.Context, then composites the result
// to the surface. A shell whose resolved renderer is GPU returns one once its
// GPU canvas is ready; otherwise it returns a CPU PixelTarget and present()
// takes the CPU path below. Selection is per-frame, so a shell can serve CPU
// frames while its GPU device is still initializing, then switch over.
type gpuCanvasTarget interface {
	RenderGPU(replay func(*gg.Context))
}

// present rasterizes the recorded scene for one frame. When the frame offers a
// GPU target the whole scene is replayed on the GPU each frame; otherwise the
// CPU rasterizer paints the damaged region and hands finished pixels to the
// shell.
func (h *shellHandler) present(f shell.Frame, changed bool, damage geom.Rect) {
	if gt, ok := f.Target().(gpuCanvasTarget); ok {
		gt.RenderGPU(func(cc *gg.Context) {
			// The GPU rasterizes the whole frame each time; replay in full.
			h.core.ReplayScene(h.core.Painter.GPUCanvas(cc))
		})
		return
	}
	if changed {
		canvas := h.core.Painter.Begin(f)
		h.core.ReplayDamaged(canvas, damage)
	}
	// Present even when skipped: the painter's surface is retained, and the
	// swapchain still needs this frame's image.
	if err := h.core.Painter.End(f); err != nil {
		log.Printf("gophics: present: %v", err)
	}
}
