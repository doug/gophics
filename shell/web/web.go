//go:build js && wasm

// Package web implements shell in the browser: a full-window <canvas>
// presented via 2D putImageData (the paint layer rasterizes on the CPU),
// frames driven by requestAnimationFrame, and DOM input events.
//
// Serve the wasm binary with wasm_exec.js (see examples/*/web). WebGPU
// presentation can replace the 2D blit later without touching anything
// above the shell.
package web

import (
	"errors"
	"image"
	"syscall/js"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// Run attaches a canvas to the document body and drives h forever.
func Run(h shell.Handler, cfg shell.Config) error {
	doc := js.Global().Get("document")
	if cfg.Title != "" {
		doc.Set("title", cfg.Title)
	}
	canvas := doc.Call("createElement", "canvas")
	canvas.Get("style").Set("cssText", "width:100vw;height:100vh;display:block;margin:0;cursor:default")
	doc.Get("body").Get("style").Set("cssText", "margin:0;overflow:hidden")
	doc.Get("body").Call("appendChild", canvas)
	ctx2d := canvas.Call("getContext", "2d")

	w := &window{canvas: canvas, ctx2d: ctx2d, doc: doc, handler: h}
	w.resize()

	listen := func(target js.Value, event string, fn func(e js.Value)) {
		target.Call("addEventListener", event, js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0])
			return nil
		}))
	}

	pos := func(e js.Value) geom.Pt {
		return geom.Pt{
			X: float32(e.Get("clientX").Float()),
			Y: float32(e.Get("clientY").Float()),
		}
	}
	listen(canvas, "mousemove", func(e js.Value) {
		h.Event(w, shell.Pointer{Kind: shell.PointerMove, Pos: pos(e)})
	})
	listen(canvas, "mousedown", func(e js.Value) {
		h.Event(w, shell.Pointer{Kind: shell.PointerDown, Pos: pos(e), Button: uint8(e.Get("button").Int())})
	})
	listen(canvas, "mouseup", func(e js.Value) {
		h.Event(w, shell.Pointer{Kind: shell.PointerUp, Pos: pos(e), Button: uint8(e.Get("button").Int())})
	})
	listen(canvas, "wheel", func(e js.Value) {
		e.Call("preventDefault")
		h.Event(w, shell.Pointer{Kind: shell.PointerScroll, Scroll: geom.Pt{
			X: -float32(e.Get("deltaX").Float()),
			Y: -float32(e.Get("deltaY").Float()),
		}})
	})
	listen(doc, "keydown", func(e js.Value) {
		key := e.Get("key").String()
		if code := keyCode(key); code != shell.KeyUnknown {
			e.Call("preventDefault")
			h.Event(w, shell.Key{Kind: shell.KeyPress, Code: code})
			return
		}
		// Printable input: single-rune keys without command modifiers.
		// (IME composition events come with the M7 text input work.)
		if len([]rune(key)) == 1 && !e.Get("ctrlKey").Bool() && !e.Get("metaKey").Bool() {
			h.Event(w, shell.Text{S: key})
		}
	})
	listen(doc, "keyup", func(e js.Value) {
		if code := keyCode(e.Get("key").String()); code != shell.KeyUnknown {
			h.Event(w, shell.Key{Kind: shell.KeyRelease, Code: code})
		}
	})
	listen(js.Global(), "resize", func(js.Value) {
		w.resize()
		h.Event(w, shell.Resize{Size: w.logical, Scale: float32(w.dpr)})
		w.Invalidate()
	})

	h.Event(w, shell.Resize{Size: w.logical, Scale: float32(w.dpr)})
	w.Invalidate()
	select {} // run forever; the browser owns the loop
}

func keyCode(key string) shell.KeyCode {
	switch key {
	case "Enter":
		return shell.KeyEnter
	case "Backspace":
		return shell.KeyBackspace
	case "Delete":
		return shell.KeyDelete
	case "Escape":
		return shell.KeyEscape
	case "Tab":
		return shell.KeyTab
	case "ArrowLeft":
		return shell.KeyLeft
	case "ArrowRight":
		return shell.KeyRight
	case "ArrowUp":
		return shell.KeyUp
	case "ArrowDown":
		return shell.KeyDown
	}
	return shell.KeyUnknown
}

type window struct {
	canvas, ctx2d, doc js.Value
	handler            shell.Handler

	logical    geom.Size
	dpr        float64
	rafPending bool
	rafFunc    js.Func
	lastNow    float64

	buf       js.Value // Uint8ClampedArray cache
	imageData js.Value
	bufW, bufH int
}

func (w *window) resize() {
	win := js.Global()
	w.dpr = win.Get("devicePixelRatio").Float()
	lw := win.Get("innerWidth").Float()
	lh := win.Get("innerHeight").Float()
	w.logical = geom.Size{W: float32(lw), H: float32(lh)}
	w.canvas.Set("width", int(lw*w.dpr))
	w.canvas.Set("height", int(lh*w.dpr))
}

func (w *window) Invalidate() {
	if w.rafPending {
		return
	}
	w.rafPending = true
	if w.rafFunc.IsUndefined() {
		w.rafFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
			w.rafPending = false
			now := args[0].Float() / 1000
			dt := now - w.lastNow
			if w.lastNow == 0 || dt < 0 || dt > 0.25 {
				dt = 1.0 / 60
			}
			w.lastNow = now
			w.handler.Frame(w, &frame{w: w}, dt)
			return nil
		})
	}
	js.Global().Call("requestAnimationFrame", w.rafFunc)
}

func (w *window) SetTitle(title string) { w.doc.Set("title", title) }
func (w *window) Close()                {}

func (w *window) ClipboardRead() (string, error) {
	return "", errors.New("web: synchronous clipboard read unsupported")
}

func (w *window) ClipboardWrite(text string) error {
	js.Global().Get("navigator").Get("clipboard").Call("writeText", text)
	return nil
}

func (w *window) put(img *image.RGBA) {
	pw, ph := img.Rect.Dx(), img.Rect.Dy()
	if w.buf.IsUndefined() || w.bufW != pw || w.bufH != ph {
		w.buf = js.Global().Get("Uint8ClampedArray").New(len(img.Pix))
		w.imageData = js.Global().Get("ImageData").New(w.buf, pw, ph)
		w.bufW, w.bufH = pw, ph
	}
	js.CopyBytesToJS(w.buf, img.Pix)
	w.ctx2d.Call("putImageData", w.imageData, 0, 0)
}

type frame struct {
	w *window
}

func (f *frame) Size() geom.Size { return f.w.logical }
func (f *frame) Scale() float32  { return float32(f.w.dpr) }
func (f *frame) Target() shell.Target {
	return shell.PixelTarget{Put: f.w.put}
}
