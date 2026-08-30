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

// setSizeOnMain queues the resize and reports acceptance rather than outcome.
//
// AppKit's setFrame must run on the main thread, and by the time it does this
// call has returned, so the real answer cannot be reported without a data race.
// AppKit accepts programmatic resize on every window gophics creates, so "true"
// here is the platform's answer and not a placeholder — unlike Wayland, which
// refuses and runs inline to say so.
func (w *window) setSizeOnMain(width, height int) bool {
	w.runOnMain(func() { w.app.SetSize(width, height) })
	return true
}
