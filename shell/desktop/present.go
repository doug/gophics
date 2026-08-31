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

	"github.com/doug/gophics/geom"
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
	return shell.PixelTarget{Put: f.w.putCPU(f.dc)}
}

// putCPU uploads the painter's surface and presents it, sending only the
// damaged rows when it can.
//
// The texture is retained across frames rather than created per frame. It used
// to be a fresh NewTextureFromImage every frame with the old one deferred for
// destruction, which had no destination to update a sub-rect of, so the damage
// rect this signature receives was discarded — and this path is every frame
// under the CPU renderer, not an edge case.
//
// The damage rect arrives in the surface's physical pixels — app.present
// scales it, because that is where logical coordinates end.
//
// Rows, not a sub-rect. The surface is row-major, so a row range is one
// contiguous span and one upload; a sub-rect's columns would be a WriteTexture
// per row, which costs more calls than it saves bytes. shell/web's putCPU made
// the same call for the same reason.
//
// A retained texture is also the reason the deferred destroy can go: the GPU
// may still be sampling the texture when this returns, and previously that
// meant the *fresh* texture had to outlive the frame. Now the same texture is
// reused, so there is nothing to free — and the region written next frame is
// written before that frame's draw samples it.
func (w *window) putCPU(dc *gogpu.Context) func(*image.RGBA, geom.Rect) {
	return func(img *image.RGBA, damage geom.Rect) {
		r := dc.Renderer()
		pw, ph := img.Rect.Dx(), img.Rect.Dy()

		fresh := w.cpuTex == nil || w.cpuTexW != pw || w.cpuTexH != ph
		if fresh {
			if w.cpuTex != nil {
				r.EnqueueDeferredDestroy(w.cpuTex.Destroy)
			}
			tex, err := r.NewTextureFromImage(img)
			if err != nil {
				log.Printf("gophics/desktop: upload frame: %v", err)
				return
			}
			w.cpuTex, w.cpuTexW, w.cpuTexH = tex, pw, ph
		} else {
			y0, y1 := damageRows(damage, ph)
			if y1 > y0 {
				lo := y0 * img.Stride
				hi := y1 * img.Stride
				if err := w.cpuTex.UpdateRegion(0, y0, pw, y1-y0, img.Pix[lo:hi]); err != nil {
					log.Printf("gophics/desktop: update frame region: %v", err)
					return
				}
			}
		}

		if err := dc.PresentTexture(w.cpuTex); err != nil {
			log.Printf("gophics/desktop: present: %v", err)
		}
	}
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

// damageRows converts a physical-pixel damage rect to the row range to upload,
// clamped to the surface. An empty range means nothing needs uploading: the
// retained texture still holds the previous frame, so it is presented again.
func damageRows(damage geom.Rect, height int) (lo, hi int) {
	lo, hi = int(damage.Min.Y), int(damage.Max.Y)
	lo = max(lo, 0)
	hi = min(hi, height)
	if hi <= lo {
		return 0, 0
	}
	return lo, hi
}
