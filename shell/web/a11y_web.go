//go:build js && wasm

// Web implementation of the shell accessibility capability (shell/a11y.go).
// Announce speaks through an off-screen aria-live region — the standard way to
// push transient messages to a screen reader. SetTree (full ARIA-mirror DOM) is
// the larger piece and is left as documented future work.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

func (w *window) Accessibility() shell.Accessibility { return &webA11y{doc: w.doc} }

type webA11y struct{ doc js.Value }

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

// SetTree is not yet implemented on web: Announce covers live feedback; a full
// explorable ARIA DOM mirror is the larger accessibility piece (see
// docs/design-capabilities.md).
func (a *webA11y) SetTree(root shell.A11yNode) {}
