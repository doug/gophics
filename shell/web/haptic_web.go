//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Haptic exposes the browser's Vibration API as the platform haptic capability.
// It returns nil when navigator.vibrate is unavailable (desktop browsers, iOS
// Safari), so ctx.Haptic() is nil there — matching the "no haptics" platforms.
func (w *window) Haptic() shell.Haptic {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() || nav.Get("vibrate").Type() != js.TypeFunction {
		return nil
	}
	return webHaptic{nav: nav}
}

type webHaptic struct{ nav js.Value }

// vibratePattern maps each kind to a navigator.vibrate pattern (milliseconds,
// alternating vibrate/pause). The browser has no impact/selection distinction,
// so intensity is approximated by duration; notifications use a short burst.
func vibratePattern(k shell.HapticKind) []any {
	switch k {
	case shell.HapticSelection:
		return []any{5}
	case shell.HapticLight:
		return []any{10}
	case shell.HapticMedium:
		return []any{18}
	case shell.HapticHeavy:
		return []any{28}
	case shell.HapticSuccess:
		return []any{10, 60, 10}
	case shell.HapticWarning:
		return []any{18, 60, 18}
	case shell.HapticError:
		return []any{28, 50, 28, 50, 28}
	default:
		return []any{10}
	}
}

func (h webHaptic) Play(k shell.HapticKind) {
	p := vibratePattern(k)
	arr := js.Global().Get("Array").New(len(p))
	for i, v := range p {
		arr.SetIndex(i, v)
	}
	h.nav.Call("vibrate", arr)
}
