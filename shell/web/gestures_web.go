//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// GestureTuning follows the host platform, from the user agent, for the same
// reason ScrollPhysics does: a canvas app's taps and long presses are
// gophics's own, and they should behave like the phone's.
func (w *window) GestureTuning() shell.GestureTuning {
	ua := js.Global().Get("navigator").Get("userAgent")
	if ua.Type() == js.TypeString && strings.Contains(ua.String(), "Android") {
		return shell.AndroidGestureTuning()
	}
	return shell.IOSGestureTuning()
}
