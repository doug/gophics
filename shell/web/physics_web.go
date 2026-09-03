//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// ScrollPhysics is the host platform's fling curve, from the user agent. A
// canvas app gets no momentum from the browser — the page's own scroll has it,
// the canvas does not — so the fling is gophics's, and it should feel like the
// phone it is running on: an Android user's thumb expects OverScroller, an
// iPhone user's expects UIKit. Everything that is not Android gets UIKit's
// curve, desktops with touchscreens included; that was the only curve before
// this existed and nothing has measured a better answer for them.
func (w *window) ScrollPhysics() shell.ScrollPhysics {
	ua := js.Global().Get("navigator").Get("userAgent")
	if ua.Type() == js.TypeString && strings.Contains(ua.String(), "Android") {
		return shell.AndroidScrollPhysics()
	}
	return shell.IOSScrollPhysics()
}
