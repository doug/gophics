//go:build js && wasm

// Web implementation of the shell accessibility capability (shell/a11y.go).
//
// Announce speaks through an off-screen aria-live region — the standard way to
// push transient messages to a screen reader. SetTree maintains an ARIA DOM
// mirror: one absolutely-positioned element per semantic node, layered over the
// canvas, carrying the role and label. Because gophics paints into a canvas,
// this mirror is the only thing a screen reader can see; the canvas itself is
// marked aria-hidden so the AT reads the mirror and not an opaque graphic.
//
// The mirror is transparent to the pointer (pointer-events: none) so ordinary
// mouse and touch input still reaches the canvas and flows through gophics's
// own hit testing. Assistive technology activates a node through the
// accessibility API rather than a real pointer, which arrives here as a click
// or keydown on the element and is routed back by ID.
//
// # Activation and the user-gesture window
//
// One asymmetry is worth knowing about. Ordinary pointer events run the widget
// handler synchronously inside the DOM listener (see web.go), so anything that
// needs a user gesture — the file picker and camera capture both call
// input.click() and say so — runs while the gesture is live. Activation from
// an AT does not: app.wireCapabilities wraps the bridge so the callback is
// posted to the UI goroutine, which runs it at the top of the next frame.
//
// That is still inside the gesture window. Transient activation lives for five
// seconds in Chrome, against roughly sixteen milliseconds for a frame, so a
// picker opened by a screen-reader activation works the same as one opened by
// a tap. It is a wide margin, but it is a dependency rather than a guarantee:
// if a browser ever tightened transient activation to the dispatching task,
// AT-driven file pickers would silently stop opening while mouse-driven ones
// kept working. Reasoned from the spec, not yet measured in a browser.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

func (w *window) Accessibility() shell.Accessibility { return &webA11y{doc: w.doc, win: w} }

type webA11y struct {
	doc js.Value
	win *window
	// host is the container holding the mirror elements.
	host js.Value
	// els maps node ID → mirror element for the current tree, so the second
	// pass can parent each node under its parent. The tree is rebuilt rather
	// than patched: the app layer only republishes on real semantic change,
	// and a rebuild cannot drift out of sync with the widget tree the way a
	// patch can. The cost is that DOM focus is dropped, which is why the node
	// gophics considers focused is re-focused explicitly below.
	els map[int]js.Value
	// funcs are the event callbacks currently attached, released on rebuild.
	funcs []js.Func
}

// region returns (creating once) the off-screen aria-live element for a politeness.
func (a *webA11y) region(assertive bool) js.Value {
	id, politeness := "gophics-aria-polite", "polite"
	if assertive {
		id, politeness = "gophics-aria-assertive", "assertive"
	}
	el := a.doc.Call("getElementById", id)
	if el.IsNull() {
		el = a.doc.Call("createElement", "div")
		el.Set("id", id)
		el.Call("setAttribute", "aria-live", politeness)
		el.Call("setAttribute", "aria-atomic", "true")
		for k, v := range map[string]string{
			"position": "fixed", "left": "-9999px", "top": "0",
			"width": "1px", "height": "1px", "overflow": "hidden",
		} {
			el.Get("style").Set(k, v)
		}
		a.doc.Get("body").Call("appendChild", el)
	}
	return el
}

func (a *webA11y) Announce(message string, assertive bool) {
	el := a.region(assertive)
	// Clear first so an identical repeated message still triggers the AT.
	el.Set("textContent", "")
	el.Set("textContent", message)
}

// container returns (creating once) the mirror host, positioned over the canvas.
func (a *webA11y) container() js.Value {
	if !a.host.IsUndefined() && !a.host.IsNull() {
		return a.host
	}
	host := a.doc.Call("getElementById", "gophics-a11y")
	if host.IsNull() {
		host = a.doc.Call("createElement", "div")
		host.Set("id", "gophics-a11y")
		for k, v := range map[string]string{
			"position": "absolute", "left": "0", "top": "0",
			"width": "0", "height": "0", "overflow": "visible",
			// The mirror must never eat a real pointer event; children inherit
			// this and re-enable nothing.
			"pointerEvents": "none",
		} {
			host.Get("style").Set(k, v)
		}
		a.doc.Get("body").Call("appendChild", host)
	}
	a.host = host
	return host
}

// SetTree rebuilds the ARIA mirror. The app layer already suppresses
// republishes for an unchanged tree, so this runs only on real semantic change.
func (a *webA11y) SetTree(nodes []shell.A11yNode, activate func(id int)) {
	host := a.container()

	// Release the previous frame's callbacks. js.Func leaks its Go closure
	// until Release, so this is not optional bookkeeping.
	for _, f := range a.funcs {
		f.Release()
	}
	a.funcs = a.funcs[:0]
	host.Set("innerHTML", "")
	a.els = make(map[int]js.Value, len(nodes))

	if len(nodes) == 0 {
		a.setCanvasHidden(false)
		return
	}
	// With a mirror in place the canvas is decoration: let the AT read the
	// mirror instead of announcing an unlabeled graphic.
	a.setCanvasHidden(true)

	scale := a.win.dpr
	if scale <= 0 {
		scale = 1
	}
	for _, n := range nodes {
		a.els[n.ID] = a.element(n, scale, activate)
	}
	// Parent each element under its parent's element so the AT walks the same
	// hierarchy the widget tree has; orphans fall back to the host.
	known := make(map[int]bool, len(nodes))
	for _, n := range nodes {
		known[n.ID] = true
	}
	for _, n := range nodes {
		parent := host
		if id := parentOf(n, known); id != "" {
			parent = a.els[n.ParentID]
		}
		parent.Call("appendChild", a.els[n.ID])
	}
}

// element applies a described mirror node to the DOM. Every decision was made
// by describeNode (a11y_mirror.go); this only carries it across the bridge.
func (a *webA11y) element(n shell.A11yNode, scale float64, activate func(id int)) js.Value {
	d := describeNode(n, scale)
	el := a.doc.Call("createElement", d.Tag)
	el.Set("id", d.ID)
	for k, v := range d.Attrs {
		el.Call("setAttribute", k, v)
	}
	style := el.Get("style")
	for k, v := range d.Style {
		style.Set(k, v)
	}
	if d.Text != "" {
		el.Set("textContent", d.Text)
	}
	if d.Clickable && activate != nil {
		id := n.ID
		fn := js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) > 0 {
				args[0].Call("preventDefault")
				args[0].Call("stopPropagation")
			}
			activate(id)
			return nil
		})
		a.funcs = append(a.funcs, fn)
		el.Call("addEventListener", "click", fn)
	}
	if d.Focus {
		// Keep the AT's focus on the node gophics considers focused, so
		// tabbing through the mirror and typing into the app agree.
		el.Call("focus", map[string]any{"preventScroll": true})
	}
	return el
}

// setCanvasHidden marks the drawing surface as decoration (or restores it) so
// the AT reads the mirror instead of an unlabeled canvas.
func (a *webA11y) setCanvasHidden(hidden bool) {
	c := a.win.canvas
	if c.IsUndefined() || c.IsNull() {
		return
	}
	if hidden {
		c.Call("setAttribute", "aria-hidden", "true")
	} else {
		c.Call("removeAttribute", "aria-hidden")
	}
}
