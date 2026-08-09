//go:build js && wasm

// Web implementation of the shell lifecycle capability (shell/lifecycle.go),
// driven by the Page Visibility API (document.visibilitychange) plus window
// focus/blur:
//
//   - document.hidden true  → StateBackground (tab hidden / minimized)
//   - document visible + window focused → StateActive
//   - document visible but blurred (another window/app frontmost) → StateInactive
//
// visibilitychange is available in every browser, so this capability is always
// returned on web. The listeners live for the page lifetime, so their js.Funcs
// are intentionally not released.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Lifecycle makes the web window a shell.LifecycleWindow.
func (w *window) Lifecycle() shell.Lifecycle {
	lc := &webLifecycle{doc: w.doc, state: shell.StateActive}
	lc.wire()
	return lc
}

type webLifecycle struct {
	doc     js.Value
	state   shell.AppState
	focused bool // last-seen window focus, to disambiguate Active vs Inactive
	cbs     []func(shell.AppState)
}

func (l *webLifecycle) State() shell.AppState { return l.state }

func (l *webLifecycle) OnChange(f func(shell.AppState)) {
	if f == nil {
		return
	}
	l.cbs = append(l.cbs, f)
}

// wire subscribes to the visibility/focus events and seeds the initial state
// from document.visibilityState and document.hasFocus(). Called once, when the
// app runner asks the window for its Lifecycle.
func (l *webLifecycle) wire() {
	l.focused = true
	if hf := l.doc.Get("hasFocus"); hf.Truthy() {
		l.focused = l.doc.Call("hasFocus").Bool()
	}
	l.state = l.compute() // seed from current visibility/focus; no callbacks yet

	l.doc.Call("addEventListener", "visibilitychange", js.FuncOf(func(js.Value, []js.Value) any {
		l.recompute()
		return nil
	}))
	win := js.Global().Get("window")
	if win.IsUndefined() {
		return
	}
	win.Call("addEventListener", "focus", js.FuncOf(func(js.Value, []js.Value) any {
		l.focused = true
		l.recompute()
		return nil
	}))
	win.Call("addEventListener", "blur", js.FuncOf(func(js.Value, []js.Value) any {
		l.focused = false
		l.recompute()
		return nil
	}))
}

// compute derives the current state from visibility + focus.
func (l *webLifecycle) compute() shell.AppState {
	if vs := l.doc.Get("visibilityState"); vs.Type() == js.TypeString && vs.String() == "hidden" {
		return shell.StateBackground
	}
	if l.doc.Get("hidden").Truthy() {
		return shell.StateBackground
	}
	if !l.focused {
		return shell.StateInactive
	}
	return shell.StateActive
}

// recompute updates state and fans the new value out to callbacks on a change.
func (l *webLifecycle) recompute() {
	s := l.compute()
	if s == l.state {
		return
	}
	l.state = s
	for _, cb := range l.cbs {
		cb(s)
	}
}
