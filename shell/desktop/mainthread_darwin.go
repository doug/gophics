//go:build darwin && !ios && !js

package desktop

import "github.com/doug/gophics/internal/objc"

// runOnMain runs fn on the macOS main thread: inline when the caller already is
// the main thread, otherwise queued for the next OnUpdate. See mainthread.go.
func (w *window) runOnMain(fn func()) {
	if fn == nil {
		return
	}
	if objc.Init() == nil && objc.Class("NSThread").SendBool("isMainThread") {
		fn()
		return
	}
	w.queueMain(fn)
}
