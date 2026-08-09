//go:build js && wasm

// Web implementation of the shell geolocation capability (shell/geolocation.go).
// Current maps to navigator.geolocation.getCurrentPosition and Watch to
// watchPosition. The success/error js.Funcs are released once they can no longer
// fire: a one-shot Current releases both on the first callback; a Watch releases
// its success func when the returned cancel runs clearWatch.

package web

import (
	"errors"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Geolocation satisfies shell.GeolocationWindow for the web shell, or returns nil
// when the browser exposes no geolocation API (e.g. an insecure context).
func (w *window) Geolocation() shell.Geolocation {
	geo := js.Global().Get("navigator").Get("geolocation")
	if geo.IsUndefined() {
		return nil
	}
	return webGeolocation{geo: geo}
}

type webGeolocation struct{ geo js.Value }

// Current requests one position fix. Exactly one of ok/fail fires; both funcs are
// released there so neither leaks.
func (g webGeolocation) Current(cb func(lat, lon, accuracy float64, err error)) {
	var ok, fail js.Func
	ok = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ok.Release()
		fail.Release()
		c := args[0].Get("coords")
		cb(c.Get("latitude").Float(), c.Get("longitude").Float(), c.Get("accuracy").Float(), nil)
		return nil
	})
	fail = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ok.Release()
		fail.Release()
		msg := "geolocation: position unavailable"
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			if m := args[0].Get("message"); m.Type() == js.TypeString {
				msg = m.String()
			}
		}
		cb(0, 0, 0, errors.New(msg))
		return nil
	})
	g.geo.Call("getCurrentPosition", ok, fail)
}

// Watch subscribes to position updates and returns a cancel that stops the watch
// and releases the callback func.
func (g webGeolocation) Watch(cb func(lat, lon, accuracy float64)) (cancel func()) {
	var success js.Func
	success = js.FuncOf(func(_ js.Value, args []js.Value) any {
		c := args[0].Get("coords")
		cb(c.Get("latitude").Float(), c.Get("longitude").Float(), c.Get("accuracy").Float())
		return nil
	})
	id := g.geo.Call("watchPosition", success)
	return func() {
		g.geo.Call("clearWatch", id)
		success.Release()
	}
}
