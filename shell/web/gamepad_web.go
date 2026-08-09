//go:build js && wasm

// Web implementation of the shell gamepad capability (shell/gamepad.go). Each
// Poll() snapshots navigator.getGamepads() — the browser Gamepad API is itself
// poll-style (state is read per frame, not event-dispatched), so this maps
// straight through with no callbacks. getGamepads() returns a fixed-length array
// with null holes for empty slots; we skip the holes and map the live entries.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Gamepads satisfies shell.GamepadWindow for the web shell.
func (w *window) Gamepads() shell.Gamepads { return webGamepads{} }

type webGamepads struct{}

// Poll snapshots the currently connected controllers. When no controller is
// attached — the common case — it returns an empty slice, never nil-derefs, so a
// game can call it unconditionally every frame. This is verifiable in a browser
// with no hardware: it must not panic and yields an empty result.
func (webGamepads) Poll() []shell.Gamepad {
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() || nav.Get("getGamepads").IsUndefined() {
		return nil
	}
	list := nav.Call("getGamepads")
	n := list.Length()
	out := make([]shell.Gamepad, 0, n)
	for i := 0; i < n; i++ {
		gp := list.Index(i)
		if gp.IsNull() || gp.IsUndefined() {
			continue // empty controller slot
		}

		buttons := gp.Get("buttons")
		bn := buttons.Length()
		bs := make([]float32, bn)
		for j := 0; j < bn; j++ {
			bs[j] = float32(buttons.Index(j).Get("value").Float())
		}

		axes := gp.Get("axes")
		an := axes.Length()
		ax := make([]float32, an)
		for j := 0; j < an; j++ {
			ax[j] = float32(axes.Index(j).Float())
		}

		out = append(out, shell.Gamepad{
			ID:        gp.Get("id").String(),
			Buttons:   bs,
			Axes:      ax,
			Connected: gp.Get("connected").Bool(),
		})
	}
	return out
}
