//go:build js && wasm

package web

import "github.com/doug/gophics/shell"

// GestureTuning follows the host platform, from the user agent, for the same
// reason ScrollPhysics does: a canvas app's taps and long presses are
// gophics's own, and they should behave like the phone's.
func (w *window) GestureTuning() shell.GestureTuning {
	if hostIsAndroid() {
		return shell.AndroidGestureTuning()
	}
	// Touch values are UIKit's everywhere that is not Android; the key
	// convention follows the host: a Mac browser, an iPhone, an iPad get
	// Cmd/Alt, a Windows or Linux browser gets Ctrl.
	g := shell.IOSGestureTuning()
	g.MacKeys = hostIsApple()
	return g
}
