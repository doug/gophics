//go:build !js

// Presentation for the desktop shell. The renderer is chosen at runtime from
// Config.Renderer (resolved by the app layer): the GPU path rasterizes each
// frame on the GPU via gg's accelerator (ggcanvas) and composites straight to
// the swapchain; the CPU path uploads a CPU-rasterized image as a texture and
// draws it fullscreen. A GPU frame is served only once the GPU canvas exists,
// so early frames (and the CPU renderer) fall through to the texture blit.
package desktop

import (
	"image"
	"log"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/integration/ggcanvas"
	"github.com/doug/gophics/internal/gfx/gogpu"

	"github.com/doug/gophics/shell"
)

// onFrameStart lazily creates (and keeps sized) the GPU canvas when the
// resolved renderer wants the GPU. It is a no-op for the CPU renderer.
func (w *window) onFrameStart(dc *gogpu.Context) {
	if w.renderer == shell.RendererCPU {
		return
	}
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
		log.Printf("gophics/desktop: ggcanvas init: %v", err)
		return
	}
	w.ggc = c
}

// Target returns this frame's presentation target: a GPU target when the GPU
// canvas is ready, otherwise a CPU PixelTarget (the initial frames before the
// GPU canvas exists, and every frame under the CPU renderer). The GPU target
// is a per-window singleton rebound to the current frame's context — the app
// layer uses its identity to recognize "same surface as the frame I last
// rendered" when deciding whether an unchanged scene can skip the GPU replay.
func (f *frame) Target() shell.Target {
	if f.w.renderer != shell.RendererCPU {
		if c, ok := f.w.ggc.(*ggcanvas.Canvas); ok {
			if f.w.gpuT == nil || f.w.gpuT.ggc != c {
				f.w.gpuT = &gpuTarget{ggc: c}
			}
			f.w.gpuT.dc = f.dc
			return f.w.gpuT
		}
		// GPU canvas not ready yet; fall through to the CPU blit for now.
	}
	return shell.PixelTarget{Put: func(img *image.RGBA) {
		r := f.dc.Renderer()
		tex, err := r.NewTextureFromImage(img)
		if err != nil {
			log.Printf("gophics/desktop: upload frame: %v", err)
			return
		}
		// PresentTexture submits an async GPU draw that samples tex; the GPU
		// may still be reading it after this returns. Defer destruction to the
		// next frame's BeginFrame (after the GPU consumed it) — destroying it
		// now freed it mid-flight, causing trailing streaks under slow motion.
		if err := f.dc.PresentTexture(tex); err != nil {
			log.Printf("gophics/desktop: present: %v", err)
		}
		r.EnqueueDeferredDestroy(tex.Destroy)
	}}
}

// gpuTarget carries the GPU canvas and this frame's swapchain context; the app
// layer type-asserts it (app.gpuCanvasTarget) and drives the scene replay. One
// instance lives per window (identity-stable across frames — see Target); dc is
// rebound to the live gogpu.Context each frame.
type gpuTarget struct {
	ggc *ggcanvas.Canvas
	dc  *gogpu.Context
}

// RenderGPU runs the scene replay against the GPU-backed gg.Context, then
// composites the result to the swapchain.
func (t *gpuTarget) RenderGPU(replay func(*gg.Context)) {
	_ = t.ggc.Draw(func(cc *gg.Context) { replay(cc) })
	if err := t.ggc.Render(t.dc.RenderTarget()); err != nil {
		log.Printf("gophics/desktop: gpu render: %v", err)
	}
}

// SkipRenderGPU (app.gpuSkipTarget) marks this frame as deliberately rendering
// nothing: the scene is unchanged, so the last presented frame stays on screen.
// gogpu's lazy swapchain acquire means no draw call → no acquire and no
// present; without this marker it would treat the workless frame as a failed
// begin and schedule a recovery redraw, spinning the app at vsync while idle.
func (t *gpuTarget) SkipRenderGPU() { t.dc.SkipFrame() }
