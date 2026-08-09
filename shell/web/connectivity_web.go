//go:build js && wasm

// Web implementation of the shell connectivity capability
// (shell/connectivity.go) using navigator.onLine and the window online/offline
// events.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Connectivity reports network status from navigator.onLine and drives OnChange
// off the window online/offline events. navigator.onLine is available in every
// browser, so this is always returned on web.
func (w *window) Connectivity() shell.Connectivity { return &webConnectivity{} }

type webConnectivity struct {
	cbs   []func(bool)
	wired bool // online/offline listeners registered lazily on first OnChange
}

func (c *webConnectivity) Online() bool {
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() {
		return true
	}
	ol := nav.Get("onLine")
	if ol.IsUndefined() {
		return true // unknown: assume online rather than falsely gating offline
	}
	return ol.Bool()
}

func (c *webConnectivity) OnChange(f func(bool)) {
	if f == nil {
		return
	}
	c.cbs = append(c.cbs, f)
	if c.wired {
		return
	}
	c.wired = true
	win := js.Global().Get("window")
	if win.IsUndefined() {
		return
	}
	// One listener pair fans out to every registered callback (including any
	// added after wiring, since fire ranges over the live slice). The js.Funcs
	// live for the page lifetime, so they are intentionally not released.
	fire := func(online bool) {
		for _, cb := range c.cbs {
			cb(online)
		}
	}
	win.Call("addEventListener", "online", js.FuncOf(func(js.Value, []js.Value) any {
		fire(true)
		return nil
	}))
	win.Call("addEventListener", "offline", js.FuncOf(func(js.Value, []js.Value) any {
		fire(false)
		return nil
	}))
}
