//go:build js && wasm

// Web implementation of the shell battery capability (shell/battery.go) using
// the Battery Status API (navigator.getBattery). The API is absent in Safari
// and Firefox, where Battery() correctly returns nil.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Battery returns the battery capability, or nil when the browser lacks the
// Battery Status API (Safari, Firefox) — the correct nil-where-unsupported
// signal so callers hide any battery affordance.
func (w *window) Battery() shell.Battery {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() || nav.Get("getBattery").Type() != js.TypeFunction {
		return nil
	}
	b := &webBattery{}
	b.resolve(nav)
	return b
}

// webBattery wraps a BatteryManager. getBattery() is async, but the capability
// getter must return synchronously, so we resolve the promise in the background
// and read the live manager once it lands. Until then Level()/Charging() report
// neutral defaults (full, charging) so nothing shows a spurious low-battery
// state in the sub-millisecond window before the promise settles.
type webBattery struct {
	mgr   js.Value // resolved BatteryManager; undefined until getBattery settles
	ready bool
	cbs   []func()
}

func (b *webBattery) resolve(nav js.Value) {
	promise := nav.Call("getBattery")
	go func() {
		mgr, err := await(promise)
		if err != nil {
			return
		}
		b.mgr, b.ready = mgr, true
		notify := js.FuncOf(func(js.Value, []js.Value) any {
			for _, cb := range b.cbs {
				cb()
			}
			return nil
		})
		mgr.Call("addEventListener", "levelchange", notify)
		mgr.Call("addEventListener", "chargingchange", notify)
		// Deliver one change now so callbacks registered before the manager
		// resolved pick up the initial real values.
		for _, cb := range b.cbs {
			cb()
		}
	}()
}

func (b *webBattery) Level() float32 {
	if !b.ready {
		return 1
	}
	return float32(b.mgr.Get("level").Float())
}

func (b *webBattery) Charging() bool {
	if !b.ready {
		return true
	}
	return b.mgr.Get("charging").Bool()
}

func (b *webBattery) OnChange(f func()) {
	if f == nil {
		return
	}
	b.cbs = append(b.cbs, f)
}
