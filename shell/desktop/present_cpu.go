//go:build !gossamer_gpu

package desktop

import (
	"image"
	"log"

	"github.com/gogpu/gogpu"

	"github.com/doug/gossamer/shell"
)

// onFrameStart is a no-op in the CPU build.
func (w *window) onFrameStart(*gogpu.Context) {}

// Target presents CPU-rasterized frames by uploading them as a GPU texture
// and drawing it fullscreen (gogpu's universal PresentTexture path) — the M1
// model, shared with the web and mobile shells. Build with -tags gossamer_gpu
// for GPU rasterization (present_gpu.go).
func (f *frame) Target() shell.Target {
	return shell.PixelTarget{Put: func(img *image.RGBA) {
		r := f.dc.Renderer()
		tex, err := r.NewTextureFromImage(img)
		if err != nil {
			log.Printf("gossamer/desktop: upload frame: %v", err)
			return
		}
		// PresentTexture submits an async GPU draw that samples tex; the GPU
		// may still be reading it after this returns. Defer destruction to the
		// next frame's BeginFrame (after the GPU consumed it) — destroying it
		// now freed it mid-flight, causing trailing streaks under slow motion.
		if err := f.dc.PresentTexture(tex); err != nil {
			log.Printf("gossamer/desktop: present: %v", err)
		}
		r.EnqueueDeferredDestroy(tex.Destroy)
	}}
}
