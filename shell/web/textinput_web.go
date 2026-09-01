//go:build js && wasm

// Web implementation of the shell text-input capability (shell/textinput.go).
// gophics draws its own editor, so to raise the mobile soft keyboard we focus a
// hidden <input> and forward its input/composition/edit-key events. The input is
// a commit funnel: each `input` event yields committed text and is cleared.
//
// One element, created once and reused. Recreating it per field looked tidier
// and broke moving between fields on Chrome for Android: focus moves as blur
// (old field) then focus (new field), so the element was removed from the DOM
// and a fresh one inserted and focused inside a single gesture — and a browser
// will not raise the keyboard for an element that was not there when the
// gesture began. The keyboard went down on the first field and never came back
// up for the second.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

func (w *window) TextInput() shell.TextInput { return &webTextInput{doc: w.doc} }

type webTextInput struct {
	doc   js.Value
	input js.Value
	funcs []js.Func
	// h is the handler the *current* field registered. The DOM listeners are
	// attached once and read it, so switching fields swaps a pointer rather
	// than rebuilding the element.
	h shell.TextInputHandler
	// blurTimer defers the blur so that a Hide immediately followed by a Show
	// — which is exactly how focus moves between two fields — never actually
	// blurs. Without the defer the keyboard visibly drops and comes back, and
	// on Chrome for Android often just drops.
	blurTimer js.Value
	active    bool
}

// ensure creates the hidden input on first use and wires its listeners once.
func (t *webTextInput) ensure() {
	if !t.input.IsUndefined() && t.input.Truthy() {
		return
	}
	in := t.doc.Call("createElement", "input")
	in.Set("type", "text")

	// Invisible but focusable — display:none and visibility:hidden cannot take
	// focus, so this is opacity instead.
	//
	// Pinned to the *top*, which matters more than it looks. A phone keyboard
	// shrinks the visual viewport without changing the layout viewport, so an
	// element at bottom:0 ends up behind the keyboard — and focusing something
	// the browser thinks is off-screen makes it scroll to reveal it. That
	// scroll fires visualViewport's scroll event, which this shell treats as a
	// resize, so raising the keyboard could put the canvas into a
	// resize-and-scroll fight with the browser for the length of the keyboard
	// animation. At top:0 the element is already in view and revealing it is a
	// no-op.
	for k, v := range map[string]string{
		"position": "fixed", "opacity": "0", "left": "0", "top": "0",
		"width": "1px", "height": "1px", "border": "0", "padding": "0", "zIndex": "-1",
	} {
		in.Get("style").Set(k, v)
	}
	t.doc.Get("body").Call("appendChild", in)

	onInput := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if v := in.Get("value").String(); v != "" {
			if t.h.OnText != nil {
				t.h.OnText(v)
			}
			in.Set("value", "")
		}
		return nil
	})
	onComp := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if t.h.OnComposing != nil && len(args) > 0 {
			t.h.OnComposing(args[0].Get("data").String())
		}
		return nil
	})
	onKey := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if t.h.OnEditKey == nil || len(args) == 0 {
			return nil
		}
		switch args[0].Get("key").String() {
		case "Backspace":
			t.h.OnEditKey(shell.EditBackspace)
		case "Enter":
			t.h.OnEditKey(shell.EditEnter)
		case "ArrowLeft":
			t.h.OnEditKey(shell.EditLeft)
		case "ArrowRight":
			t.h.OnEditKey(shell.EditRight)
		}
		return nil
	})
	in.Call("addEventListener", "input", onInput)
	in.Call("addEventListener", "compositionupdate", onComp)
	in.Call("addEventListener", "keydown", onKey)

	t.input, t.funcs = in, []js.Func{onInput, onComp, onKey}
}

func (t *webTextInput) cancelBlur() {
	if t.blurTimer.Truthy() {
		js.Global().Call("clearTimeout", t.blurTimer)
		t.blurTimer = js.Undefined()
	}
}

func (t *webTextInput) Show(opts shell.TextInputOptions, h shell.TextInputHandler) {
	t.ensure()
	t.cancelBlur() // a Hide from the field being left must not take this one down
	t.h, t.active = h, true

	in := t.input
	in.Set("inputmode", inputMode(opts.Type))
	if opts.Autocorrect {
		in.Call("removeAttribute", "autocorrect")
		in.Call("removeAttribute", "autocapitalize")
		in.Set("spellcheck", true)
	} else {
		in.Set("autocorrect", "off")
		in.Set("autocapitalize", "off")
		in.Set("spellcheck", false)
	}
	in.Set("value", "")

	// Focusing an already-focused element is a no-op, which is what keeps the
	// keyboard steady when moving between fields.
	if !t.doc.Get("activeElement").Equal(in) {
		in.Call("focus") // raises the mobile soft keyboard
	}
}

func (t *webTextInput) Hide() {
	if !t.active {
		return
	}
	t.active, t.h = false, shell.TextInputHandler{}

	// Deferred: focus moving between two fields arrives as Hide then Show in
	// the same turn, and blurring in between is what drops the keyboard. A Show
	// cancels this; a real dismissal lets it run.
	t.cancelBlur()
	cb := js.FuncOf(func(js.Value, []js.Value) any {
		t.blurTimer = js.Undefined()
		if !t.active && t.input.Truthy() {
			t.input.Call("blur")
		}
		return nil
	})
	t.blurTimer = js.Global().Call("setTimeout", cb, 0)
}

// SetText is a no-op on web: the hidden input is a commit funnel (input events
// replace-and-clear), so surrounding-text context isn't needed for this path.
func (t *webTextInput) SetText(text string, selStart, selEnd int) {}

func inputMode(t shell.TextInputType) string {
	switch t {
	case shell.TextInputEmail:
		return "email"
	case shell.TextInputNumber:
		return "numeric"
	case shell.TextInputURL:
		return "url"
	case shell.TextInputSearch:
		return "search"
	default:
		return "text"
	}
}
