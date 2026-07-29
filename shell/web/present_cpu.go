//go:build js && wasm && !gossamer_gpu

package web

import (
	"image"
	"syscall/js"

	"github.com/doug/gossamer/shell"
)

// presenter blits CPU-rasterized frames to the canvas via 2D putImageData.
// This is the default web build; -tags gossamer_gpu swaps in the WebGPU
// surface presenter (present_gpu.go).
type presenter struct {
	w     *window
	ctx2d js.Value

	buf        js.Value // Uint8ClampedArray cache
	imageData  js.Value
	bufW, bufH int
}

func newPresenter(w *window) *presenter {
	return &presenter{w: w, ctx2d: w.canvas.Call("getContext", "2d")}
}

// onResize is a no-op for the CPU path: the ImageData cache re-allocates
// lazily when the frame size changes.
func (p *presenter) onResize() {}

func (p *presenter) target() shell.Target {
	return shell.PixelTarget{Put: p.put}
}

func (p *presenter) put(img *image.RGBA) {
	pw, ph := img.Rect.Dx(), img.Rect.Dy()
	if p.buf.IsUndefined() || p.bufW != pw || p.bufH != ph {
		p.buf = js.Global().Get("Uint8ClampedArray").New(len(img.Pix))
		p.imageData = js.Global().Get("ImageData").New(p.buf, pw, ph)
		p.bufW, p.bufH = pw, ph
	}
	js.CopyBytesToJS(p.buf, img.Pix)
	p.ctx2d.Call("putImageData", p.imageData, 0, 0)
}
