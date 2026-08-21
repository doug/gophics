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
	"fmt"
	"math"
	"syscall/js"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// rectPx is the canvas's position and size on screen, in CSS pixels.
type rectPx struct{ left, top, w, h float32 }

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
	if cfg.ScaleToFit {
		w.design = cfg.Size
	}
	w.resize()
	w.watchDarkMode()
	w.pres = newPresenter(w)

	listen := func(target js.Value, event string, fn func(e js.Value)) {
		target.Call("addEventListener", event, js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0])
			return nil
		}))
	}

	// Map a viewport-relative event position into the canvas's logical
	// coordinates.
	//
	// clientX/Y are relative to the viewport, not to the canvas, and the
	// canvas's displayed size is not necessarily its logical size. Passing
	// clientX/Y straight through assumes the canvas fills the viewport
	// exactly, starting at its top-left corner. When that assumption breaks
	// the input silently lands somewhere other than where it was aimed, which
	// is worse than a crash: everything still works, just not where you touch.
	//
	// Going through getBoundingClientRect covers all of it — the canvas being
	// inset by other content, the page being scrolled, a CSS size that differs
	// from the logical size, even a CSS transform — because the rect is what
	// the user is actually touching.
	// The rect is cached rather than read per event: getBoundingClientRect
	// forces the browser to flush pending layout, and pointermove fires on
	// every frame of a drag, so reading it there would mean a synchronous
	// layout per move. It is refreshed whenever the canvas can have moved --
	// on resize, on scroll, and at the start of each gesture, which is cheap
	// because presses are rare and a drag cannot begin without one.
	refreshRect := func() {
		r := canvas.Call("getBoundingClientRect")
		w.rect = rectPx{
			left: float32(r.Get("left").Float()), top: float32(r.Get("top").Float()),
			w: float32(r.Get("width").Float()), h: float32(r.Get("height").Float()),
		}
	}
	refreshRect()

	pos := func(e js.Value) geom.Pt {
		cx := float32(e.Get("clientX").Float())
		cy := float32(e.Get("clientY").Float())
		r := w.rect
		if r.w <= 0 || r.h <= 0 { // detached or display:none — nothing to map to
			return geom.Pt{X: cx, Y: cy}
		}
		return geom.Pt{
			X: (cx - r.left) * w.logical.W / r.w,
			Y: (cy - r.top) * w.logical.H / r.h,
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
		refreshRect()
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
	onResize := func(js.Value) {
		w.resize()
		refreshRect()
		h.Event(w, shell.Resize{Size: w.logical, Scale: float32(w.dpr)})
		w.Invalidate()
	}
	listen(js.Global(), "resize", onResize)
	listen(doc, "scroll", func(js.Value) { refreshRect() })
	// Mobile browsers grow and shrink the visible area as the address bar
	// hides and reveals, and raise the on-screen keyboard over it. Those change
	// visualViewport without reliably firing a window resize, so without this
	// the canvas keeps the size it had when the bar was in its other state.
	if vv := js.Global().Get("visualViewport"); vv.Truthy() {
		listen(vv, "resize", onResize)
		listen(vv, "scroll", onResize)
	}

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
	cam         *webCamera         // lazily created still-capture capability
	aud         *webAudio          // lazily created audio capability

	logical geom.Size
	// rect is the canvas's on-screen box, cached; see refreshRect.
	rect rectPx
	// design is Config.Size when Config.ScaleToFit asked for it: the size this
	// app was laid out for. When set, the app always sees exactly this logical
	// size and the canvas is scaled to fit the viewport, letterboxed. When
	// zero, the app fills the viewport and lays out responsively instead.
	design geom.Size
	// dpr is the effective device scale: devicePixelRatio, multiplied by the
	// fit factor when a design size is in play. Everything downstream derives
	// the backing store from logical*dpr, so folding fit in here is all that is
	// needed to render the scaled view crisply.
	dpr        float64
	rafPending bool
	rafFunc    js.Func
	lastNow    float64
	dark       bool // cached prefers-color-scheme, kept fresh by watchDarkMode
}

// watchDarkMode caches the platform color-scheme preference and subscribes to
// its MediaQueryList change event. The app runner polls DarkMode() every frame,
// and a matchMedia call per frame is a needless JS-bridge round trip — with the
// listener it is a plain field read, and a live preference flip still lands on
// the next frame (the listener invalidates).
func (w *window) watchDarkMode() {
	mq := js.Global().Call("matchMedia", "(prefers-color-scheme: dark)")
	if !mq.Truthy() {
		return
	}
	w.dark = mq.Get("matches").Bool()
	mq.Call("addEventListener", "change", js.FuncOf(func(_ js.Value, args []js.Value) any {
		w.dark = args[0].Get("matches").Bool()
		w.Invalidate() // repaint so the theme change takes effect now, not on the next input
		return nil
	}))
}

func (w *window) resize() {
	win := js.Global()
	w.dpr = win.Get("devicePixelRatio").Float()

	// Prefer visualViewport: on mobile it reports the area actually visible,
	// which innerHeight also does, but visualViewport keeps reporting it
	// correctly while the on-screen keyboard is up and during pinch-zoom.
	lw := win.Get("innerWidth").Float()
	lh := win.Get("innerHeight").Float()
	if vv := win.Get("visualViewport"); vv.Truthy() {
		if vw, vh := vv.Get("width").Float(), vv.Get("height").Float(); vw > 0 && vh > 0 {
			lw, lh = vw, vh
		}
	}

	// cssW/cssH are the size the canvas occupies on screen. They equal the
	// logical size unless a design size is being scaled to fit.
	cssW, cssH := lw, lh

	if w.design.W > 0 && w.design.H > 0 {
		// Scale the design size to fit, preserving its aspect ratio. A layout
		// built for a wide window stays usable on a phone held upright: it is
		// shown smaller and letterboxed rather than clipped, which is what
		// "fits on the screen" has to mean for a fixed layout.
		dw, dh := float64(w.design.W), float64(w.design.H)
		fit := math.Min(lw/dw, lh/dh)
		w.logical = w.design
		w.dpr *= fit
		cssW, cssH = dw*fit, dh*fit
	} else {
		w.logical = geom.Size{W: float32(lw), H: float32(lh)}
	}

	w.canvas.Set("width", int(float64(w.logical.W)*w.dpr))
	w.canvas.Set("height", int(float64(w.logical.H)*w.dpr))

	// Pin the displayed size explicitly. The stylesheet asks for 100vw/100vh,
	// and on mobile 100vh is the *large* viewport — the height with the address
	// bar hidden — while the measurement above is the height visible right now.
	// Those differ by roughly the address bar, so the browser stretches a frame
	// drawn for the smaller height across the taller box, and every touch lands
	// further from where it was aimed the further down the screen it is.
	// Horizontally nothing goes wrong, because 100vw and innerWidth agree —
	// which is exactly the reported symptom.
	style := w.canvas.Get("style")
	style.Set("width", fmt.Sprintf("%gpx", cssW))
	style.Set("height", fmt.Sprintf("%gpx", cssH))
	// Centre the letterbox. Harmless when the canvas fills the viewport.
	style.Set("margin", fmt.Sprintf("%gpx %gpx", math.Max(0, (lh-cssH)/2), math.Max(0, (lw-cssW)/2)))
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

// DarkMode reports the cached prefers-color-scheme value (see watchDarkMode);
// it is called every frame, so it must not cross the JS bridge.
func (w *window) DarkMode() bool { return w.dark }

type frame struct {
	w *window
}

func (f *frame) Size() geom.Size { return f.w.logical }
func (f *frame) Scale() float32  { return float32(f.w.dpr) }

// Target delegates to the build-specific presenter: a CPU PixelTarget
// (present_cpu.go) or a GPU surface target (present_gpu.go).
func (f *frame) Target() shell.Target { return f.w.pres.target() }
