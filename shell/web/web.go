//go:build js && wasm

// Package web implements shell in the browser: a full-window <canvas>,
// frames driven by requestAnimationFrame, and DOM input events.
//
// Presentation is chosen at runtime (see present.go): by default it rasterizes
// each frame on the GPU and presents to the canvas's WebGPU surface directly —
// no CPU readback — and falls back to a 2D putImageData blit when WebGPU is
// unavailable or the CPU renderer is forced (Config.Renderer). A <canvas> can
// bind only one context type for its lifetime, so the choice is committed once.
//
// Serve the wasm binary with wasm_exec.js (see examples/*/web).
package web

import (
	"errors"
	"syscall/js"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// wheelLineHeight is the logical pixels a single line-mode wheel step scrolls.
// Browsers that report deltaMode=1 give deltas in lines; ~40px per line matches
// the amount Chrome synthesizes for an equivalent notch in pixel mode.
const wheelLineHeight = 40

// Run attaches a canvas to the document body and drives h forever.
func Run(h shell.Handler, cfg shell.Config) error {
	doc := js.Global().Get("document")
	if cfg.Title != "" {
		doc.Set("title", cfg.Title)
	}
	canvas := doc.Call("createElement", "canvas")
	// touch-action:none routes touch drags to pointer events instead of the
	// browser's own scroll/zoom, so the app owns them (and can distinguish a
	// scroll drag from a selection long-press).
	canvas.Get("style").Set("cssText", "width:100vw;height:100vh;display:block;margin:0;cursor:default;touch-action:none")
	doc.Get("body").Get("style").Set("cssText", "margin:0;overflow:hidden")
	doc.Get("body").Call("appendChild", canvas)

	w := &window{canvas: canvas, doc: doc, handler: h, renderer: cfg.Renderer}
	w.resize()
	w.pres = newPresenter(w)

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
	// Pointer Events unify mouse/touch/pen and carry pointerType, so gestures
	// can adapt to the input device (mouse-drag selects; touch-drag scrolls).
	src := func(e js.Value) shell.PointerSource {
		switch e.Get("pointerType").String() {
		case "touch":
			return shell.SourceTouch
		case "pen":
			return shell.SourcePen
		default:
			return shell.SourceMouse
		}
	}
	listen(canvas, "pointermove", func(e js.Value) {
		h.Event(w, shell.Pointer{Kind: shell.PointerMove, Pos: pos(e), Source: src(e)})
	})
	listen(canvas, "pointerdown", func(e js.Value) {
		// Capture the pointer so drags keep delivering move/up even if the
		// finger/cursor leaves the canvas.
		if id := e.Get("pointerId"); !id.IsUndefined() {
			canvas.Call("setPointerCapture", id)
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerDown, Pos: pos(e), Button: uint8(e.Get("button").Int()), Source: src(e)})
	})
	listen(canvas, "pointerup", func(e js.Value) {
		h.Event(w, shell.Pointer{Kind: shell.PointerUp, Pos: pos(e), Button: uint8(e.Get("button").Int()), Source: src(e)})
	})
	listen(canvas, "wheel", func(e js.Value) {
		e.Call("preventDefault")
		// deltaMode reports the unit of deltaX/Y: 0 = pixels (trackpads and most
		// mice on macOS), 1 = lines (some mouse wheels, notably on Firefox), 2 =
		// pages. Convert lines/pages to logical pixels so a line-mode wheel
		// scrolls a sane amount instead of a few pixels per notch.
		sx, sy := float32(1), float32(1)
		switch e.Get("deltaMode").Int() {
		case 1: // lines
			sx, sy = wheelLineHeight, wheelLineHeight
		case 2: // pages
			sx, sy = w.logical.W, w.logical.H
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerScroll, Scroll: geom.Pt{
			X: -float32(e.Get("deltaX").Float()) * sx,
			Y: -float32(e.Get("deltaY").Float()) * sy,
		}})
	})
	listen(doc, "keydown", func(e js.Value) {
		key := e.Get("key").String()
		mods := modBits(e)
		if code := keyCode(key, mods); code != shell.KeyUnknown {
			e.Call("preventDefault")
			h.Event(w, shell.Key{Kind: shell.KeyPress, Code: code, Mods: mods})
			return
		}
		// Printable input: single-rune keys without command modifiers.
		// (IME composition events come with the M7 text input work.)
		if len([]rune(key)) == 1 && !e.Get("ctrlKey").Bool() && !e.Get("metaKey").Bool() {
			// The app owns text input, so suppress the browser's default action
			// for the key — most importantly Space (which otherwise scrolls the
			// page) and "/" (quick-find). Without this, typing a space in a
			// focused field scrolls instead of advancing the caret.
			e.Call("preventDefault")
			h.Event(w, shell.Text{S: key})
		}
	})
	listen(doc, "keyup", func(e js.Value) {
		mods := modBits(e)
		if code := keyCode(e.Get("key").String(), mods); code != shell.KeyUnknown {
			h.Event(w, shell.Key{Kind: shell.KeyRelease, Code: code, Mods: mods})
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

func keyCode(key string, mods shell.Mods) shell.KeyCode {
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
	case "Home":
		return shell.KeyHome
	case "End":
		return shell.KeyEnd
	}
	// Letter keys only as command shortcuts; plain letters are text input.
	if mods.Command() {
		switch key {
		case "a", "A":
			return shell.KeyA
		case "c", "C":
			return shell.KeyC
		case "v", "V":
			return shell.KeyV
		case "x", "X":
			return shell.KeyX
		}
	}
	return shell.KeyUnknown
}

func modBits(e js.Value) shell.Mods {
	var m shell.Mods
	if e.Get("shiftKey").Bool() {
		m |= shell.ModShift
	}
	if e.Get("ctrlKey").Bool() {
		m |= shell.ModCtrl
	}
	if e.Get("altKey").Bool() {
		m |= shell.ModAlt
	}
	if e.Get("metaKey").Bool() {
		m |= shell.ModSuper
	}
	return m
}

type window struct {
	canvas, doc js.Value
	handler     shell.Handler
	renderer    shell.RendererMode // resolved backend for this run
	pres        *presenter         // runtime-selected presentation (CPU blit or GPU surface)

	logical    geom.Size
	dpr        float64
	rafPending bool
	rafFunc    js.Func
	lastNow    float64
}

func (w *window) resize() {
	win := js.Global()
	w.dpr = win.Get("devicePixelRatio").Float()
	lw := win.Get("innerWidth").Float()
	lh := win.Get("innerHeight").Float()
	w.logical = geom.Size{W: float32(lw), H: float32(lh)}
	w.canvas.Set("width", int(lw*w.dpr))
	w.canvas.Set("height", int(lh*w.dpr))
	if w.pres != nil {
		w.pres.onResize()
	}
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

func (w *window) OpenURL(url string) error {
	js.Global().Call("open", url, "_blank", "noopener")
	return nil
}

func (w *window) DarkMode() bool {
	m := js.Global().Call("matchMedia", "(prefers-color-scheme: dark)")
	return m.Truthy() && m.Get("matches").Bool()
}

type frame struct {
	w *window
}

func (f *frame) Size() geom.Size { return f.w.logical }
func (f *frame) Scale() float32  { return float32(f.w.dpr) }

// Target delegates to the build-specific presenter: a CPU PixelTarget
// (present_cpu.go) or a GPU surface target (present_gpu.go).
func (f *frame) Target() shell.Target { return f.w.pres.target() }
