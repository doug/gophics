//go:build js && wasm

// Web implementation of the shell text-input capability (shell/textinput.go).
// gophics draws its own editor, so to raise the mobile soft keyboard we focus a
// hidden <input> and forward its input/composition/edit-key events. The input is
// a commit funnel: each `input` event yields committed text and is cleared.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

func (w *window) TextInput() shell.TextInput { return &webTextInput{doc: w.doc} }

type webTextInput struct {
	doc    js.Value
	input  js.Value
	funcs  []js.Func
	active bool
}

func (t *webTextInput) Show(opts shell.TextInputOptions, h shell.TextInputHandler) {
	t.Hide()

	in := t.doc.Call("createElement", "input")
	in.Set("type", "text")
	in.Set("inputmode", inputMode(opts.Type))
	if !opts.Autocorrect {
		in.Set("autocorrect", "off")
		in.Set("autocapitalize", "off")
		in.Set("spellcheck", false)
	}
	// Off-screen but focusable — display:none can't take focus / raise the keyboard.
	for k, v := range map[string]string{
		"position": "fixed", "opacity": "0", "left": "0", "bottom": "0",
		"width": "1px", "height": "1px", "border": "0", "padding": "0", "zIndex": "-1",
	} {
		in.Get("style").Set(k, v)
	}
	t.doc.Get("body").Call("appendChild", in)
	t.input, t.active = in, true

	onInput := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if v := in.Get("value").String(); v != "" {
			if h.OnText != nil {
				h.OnText(v)
			}
			in.Set("value", "")
		}
		return nil
	})
	onComp := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if h.OnComposing != nil && len(args) > 0 {
			h.OnComposing(args[0].Get("data").String())
		}
		return nil
	})
	onKey := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if h.OnEditKey == nil || len(args) == 0 {
			return nil
		}
		switch args[0].Get("key").String() {
		case "Backspace":
			h.OnEditKey(shell.EditBackspace)
		case "Enter":
			h.OnEditKey(shell.EditEnter)
		case "ArrowLeft":
			h.OnEditKey(shell.EditLeft)
		case "ArrowRight":
			h.OnEditKey(shell.EditRight)
		}
		return nil
	})
	in.Call("addEventListener", "input", onInput)
	in.Call("addEventListener", "compositionupdate", onComp)
	in.Call("addEventListener", "keydown", onKey)
	t.funcs = []js.Func{onInput, onComp, onKey}
	in.Call("focus") // raises the mobile soft keyboard
}

func (t *webTextInput) Hide() {
	if !t.active {
		return
	}
	t.input.Call("blur")
	if p := t.input.Get("parentNode"); !p.IsNull() {
		p.Call("removeChild", t.input)
	}
	for _, f := range t.funcs {
		f.Release()
	}
	t.funcs, t.active = nil, false
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
