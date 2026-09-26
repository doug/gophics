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
	"strconv"
	"strings"
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
	// The gesture model is single-pointer: a second finger on a touch screen
	// is not a second press but an interruption of the first, and forwarding
	// it as one ended the live scroll without a release and left pressed
	// highlights stuck. Only the primary pointer of each device reaches the
	// app; the others are dropped here, where the browser already says which
	// is which.
	primary := func(e js.Value) bool {
		p := e.Get("isPrimary")
		return p.IsUndefined() || p.Bool()
	}
	listen(canvas, "pointermove", func(e js.Value) {
		if !primary(e) {
			return
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerMove, Pos: pos(e), Source: src(e)})
	})
	// DOM buttons are 0 left, 1 middle, 2 right; the shell contract is
	// 0 primary, 1 secondary, 2 middle. Passing the DOM value through had
	// right-clicks arriving as "middle" and vice versa.
	button := func(e js.Value) uint8 {
		switch e.Get("button").Int() {
		case 2:
			return 1
		case 1:
			return 2
		}
		return 0
	}
	listen(canvas, "pointerdown", func(e js.Value) {
		if !primary(e) {
			return
		}
		refreshRect()
		// Capture the pointer so drags keep delivering move/up even if the
		// finger/cursor leaves the canvas.
		if id := e.Get("pointerId"); !id.IsUndefined() {
			canvas.Call("setPointerCapture", id)
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerDown, Pos: pos(e), Button: button(e), Source: src(e)})
	})
	listen(canvas, "pointerup", func(e js.Value) {
		if !primary(e) {
			return
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerUp, Pos: pos(e), Button: button(e), Source: src(e)})
	})
	// The browser cancels a pointer it has taken for itself — a touch that
	// became a native scroll or pinch, a captured pointer whose capture was
	// lost. The press ends without a tap, and without moving the pointer.
	listen(canvas, "pointercancel", func(e js.Value) {
		if !primary(e) {
			return
		}
		h.Event(w, shell.Pointer{Kind: shell.PointerCancel, Source: src(e)})
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
		// Pos matters: the app routes a scroll to whatever is under the
		// pointer, and a wheel event carries its own coordinates. Dropping
		// them left the app falling back to wherever the pointer was last
		// *moved*, so a wheel with no prior move over the window scrolled
		// nothing at all — the page that appears under a stationary cursor,
		// and every scripted interaction.
		h.Event(w, shell.Pointer{Kind: shell.PointerScroll, Pos: pos(e), Scroll: geom.Pt{
			X: -float32(e.Get("deltaX").Float()) * sx,
			Y: -float32(e.Get("deltaY").Float()) * sy,
		}})
	})
	listen(doc, "keydown", func(e js.Value) {
		if imeKey(e) {
			return // the IME owns it: Enter confirms a composition, Backspace edits it
		}
		key := e.Get("key").String()
		mods := modBits(e)
		if code := keyCode(key, mods); code != shell.KeyUnknown {
			e.Call("preventDefault")
			h.Event(w, shell.Key{Kind: shell.KeyPress, Code: code, Mods: mods})
			return
		}
		// Printable input: single-rune keys without command modifiers.
		// (Composition text arrives through the hidden input, textinput_web.go.)
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
		if imeKey(e) {
			return
		}
		mods := modBits(e)
		if code := keyCode(e.Get("key").String(), mods); code != shell.KeyUnknown {
			h.Event(w, shell.Key{Kind: shell.KeyRelease, Code: code, Mods: mods})
		}
	})
	onResize := func(js.Value) {
		changed := w.resize()
		refreshRect()
		if changed {
			h.Event(w, shell.Resize{Size: w.logical, Scale: float32(w.dpr)})
			w.Invalidate()
		}
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

// imeKey reports a key event that belongs to an active IME composition. Chrome
// and Firefox hide the real key behind "Process", but Safari reports the true
// key with isComposing set — so without this the Enter that confirms a
// Japanese composition also submitted the field, and Backspace during one also
// deleted a committed character. keyCode 229 is the older spelling of the same
// thing, still what some Android keyboards send.
func imeKey(e js.Value) bool {
	if e.Get("isComposing").Truthy() {
		return true
	}
	kc := e.Get("keyCode")
	return kc.Type() == js.TypeNumber && kc.Int() == 229
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
	// Letter keys only with a modifier held — Cmd/Ctrl for commands, Alt for
	// word movement, Ctrl for the Emacs bindings on a Mac; a plain letter is
	// text input and arrives through the input event instead.
	if mods&(shell.ModCtrl|shell.ModSuper|shell.ModAlt) != 0 && len(key) == 1 {
		c := key[0]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c >= 'A' && c <= 'Z' {
			return letterKey(c)
		}
	}
	return shell.KeyUnknown
}

// letterKey maps an ASCII uppercase letter to its KeyCode.
func letterKey(c byte) shell.KeyCode {
	switch c {
	case 'A':
		return shell.KeyA
	case 'B':
		return shell.KeyB
	case 'C':
		return shell.KeyC
	case 'D':
		return shell.KeyD
	case 'E':
		return shell.KeyE
	case 'F':
		return shell.KeyF
	case 'G':
		return shell.KeyG
	case 'H':
		return shell.KeyH
	case 'I':
		return shell.KeyI
	case 'J':
		return shell.KeyJ
	case 'K':
		return shell.KeyK
	case 'L':
		return shell.KeyL
	case 'M':
		return shell.KeyM
	case 'N':
		return shell.KeyN
	case 'O':
		return shell.KeyO
	case 'P':
		return shell.KeyP
	case 'Q':
		return shell.KeyQ
	case 'R':
		return shell.KeyR
	case 'S':
		return shell.KeyS
	case 'T':
		return shell.KeyT
	case 'U':
		return shell.KeyU
	case 'V':
		return shell.KeyV
	case 'W':
		return shell.KeyW
	case 'X':
		return shell.KeyX
	case 'Y':
		return shell.KeyY
	case 'Z':
		return shell.KeyZ
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
	spk         *webSpeakers       // lazily created audio output capability

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
	dpr float64
	// backing and box are what resize last applied to the canvas — its
	// bitmap size and its CSS size/margins — so an unchanged value is not
	// written again (writing the bitmap size clears the canvas).
	backing    geom.Size
	box        string
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

// resize re-derives the logical size, device scale and canvas box from the
// viewport, applying them to the canvas only when they changed, and reports
// whether anything did.
func (w *window) resize() (changed bool) {
	win := js.Global()
	prevLogical, prevDPR := w.logical, w.dpr
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

	// A host page can reserve a strip along the top for its own chrome. The
	// canvas is sized and positioned by this function rather than by the
	// stylesheet — see the note on pinning below — so page CSS alone cannot
	// make room, and a bar laid over the canvas collides with whatever the app
	// draws in its top-left corner.
	insetTop := hostTopInset()
	if insetTop > lh {
		insetTop = 0 // a bar taller than the viewport would leave nothing
	}
	lh -= insetTop

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

	// Only touch the canvas when something actually changed. Assigning a
	// canvas dimension clears its bitmap even when the value is the same, and
	// this runs on every visualViewport scroll as well as on resize — so a
	// pinch-zoom pan or the soft keyboard animating blanked the canvas until
	// the next frame painted, a visible flash, and sent the app a spurious
	// Resize on top.
	backing := geom.Size{W: float32(int(float64(w.logical.W) * w.dpr)), H: float32(int(float64(w.logical.H) * w.dpr))}
	// Centre the letterbox, below any reserved strip. Harmless when the canvas
	// fills the viewport and nothing is reserved.
	mv := math.Max(0, (lh-cssH)/2)
	mh := math.Max(0, (lw-cssW)/2)
	box := fmt.Sprintf("%gpx %gpx %gpx %gpx %gpx %gpx", cssW, cssH, insetTop+mv, mh, mv, mh)
	changed = w.logical != prevLogical || w.dpr != prevDPR || backing != w.backing
	if backing != w.backing {
		w.backing = backing
		w.canvas.Set("width", int(backing.W))
		w.canvas.Set("height", int(backing.H))
	}
	if box != w.box {
		w.box = box
		// Pin the displayed size explicitly. The stylesheet asks for
		// 100vw/100vh, and on mobile 100vh is the *large* viewport — the
		// height with the address bar hidden — while the measurement above is
		// the height visible right now. Those differ by roughly the address
		// bar, so the browser stretches a frame drawn for the smaller height
		// across the taller box, and every touch lands further from where it
		// was aimed the further down the screen it is. Horizontally nothing
		// goes wrong, because 100vw and innerWidth agree — which is exactly
		// the reported symptom.
		style := w.canvas.Get("style")
		style.Set("width", fmt.Sprintf("%gpx", cssW))
		style.Set("height", fmt.Sprintf("%gpx", cssH))
		style.Set("margin", fmt.Sprintf("%gpx %gpx %gpx %gpx", insetTop+mv, mh, mv, mh))
	}
	if changed && w.pres != nil {
		w.pres.onResize()
	}
	return changed
}

// canvasBox returns the canvas's on-screen box, read fresh: its top-left in
// viewport CSS pixels and the factor from logical to CSS pixels — 1 unless a
// design size is being scaled to fit. Anything laid over the canvas in the
// DOM (the accessibility mirror, a web view) has to be placed through this,
// not from logical coordinates: resize offsets the canvas by the host's top
// inset and the letterbox margins, and scales it under ScaleToFit, so raw
// logical pixels land the overlay off the content it belongs to.
func (w *window) canvasBox() (left, top, fit float64) {
	r := w.canvas.Call("getBoundingClientRect")
	left, top = r.Get("left").Float(), r.Get("top").Float()
	fit = 1
	if cw := r.Get("width").Float(); cw > 0 && w.logical.W > 0 {
		fit = cw / float64(w.logical.W)
	}
	return left, top, fit
}

// hostTopInset reads --gophics-top-inset off the root element: the height a
// host page wants kept clear at the top for its own chrome. 0 when unset, which
// is every page that has not asked for it.
//
// A CSS custom property rather than a Config field because the value belongs to
// the page, not the app — the same binary is embedded by pages that reserve
// nothing and pages that reserve a header, and only the page knows which. It is
// read every resize, so a responsive bar that changes height is followed.
func hostTopInset() float64 {
	root := js.Global().Get("document").Get("documentElement")
	if !root.Truthy() {
		return 0
	}
	style := js.Global().Call("getComputedStyle", root)
	if !style.Truthy() {
		return 0
	}
	v := strings.TrimSpace(style.Call("getPropertyValue", "--gophics-top-inset").String())
	if v == "" {
		return 0
	}
	px, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
	if err != nil || px <= 0 || math.IsNaN(px) {
		return 0
	}
	return px
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

// Close delivers Closed, as shell.Window.Close promises. A page cannot close
// itself from wasm, but the contract is about the event: an app's save-on-quit
// hook runs from it, and desktop and terminal already honour that.
func (w *window) Close() { w.handler.Event(w, shell.Closed{}) }

func (w *window) ClipboardRead() (string, error) {
	return "", errors.New("web: synchronous clipboard read unsupported")
}

func (w *window) ClipboardWrite(text string) error {
	// navigator.clipboard exists only in a secure context. Plain http:// on a
	// LAN — the usual way a phone is pointed at a dev build — leaves it
	// undefined, and Value.Call on undefined panics, which would have killed
	// the app on a copy.
	cb := js.Global().Get("navigator").Get("clipboard")
	if !cb.Truthy() {
		return errors.New("web: clipboard unavailable (insecure context?)")
	}
	// The write is a promise; it cannot fail synchronously, but an unhandled
	// rejection (the document not focused, permission denied) logs an error
	// to the console, so it is caught and dropped. One callback for both
	// outcomes: exactly one of them runs, and it releases the func.
	var settled js.Func
	settled = js.FuncOf(func(js.Value, []js.Value) any {
		settled.Release()
		return nil
	})
	cb.Call("writeText", text).Call("then", settled, settled)
	return nil
}

// OpenURL opens a tab, for the schemes shell.CheckOpenURL agrees to — the same
// rule as desktop and mobile, so a link that opens in the browser opens
// everywhere.
func (w *window) OpenURL(url string) error {
	if err := shell.CheckOpenURL(url); err != nil {
		return err
	}
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
