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
	g := shell.IOSGestureTuning()
	// Touch values are UIKit's everywhere that is not Android; the key
	// convention follows the host: a Mac browser, an iPhone, an iPad get
	// Cmd/Alt, a Windows or Linux browser gets Ctrl.
	if ua.Type() == js.TypeString {
		u := ua.String()
		g.MacKeys = strings.Contains(u, "Mac") || strings.Contains(u, "iPhone") || strings.Contains(u, "iPad")
	}
	return g
}
