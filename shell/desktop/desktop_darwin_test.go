//go:build darwin && !ios && !js

package desktop

import (
	"runtime"
	"testing"

	"github.com/doug/gophics/internal/objc"
)

// Window.SetTitle reached AppKit from whatever thread called it, and Build
// runs on the render thread, where AppKit answers a window mutation by
// aborting the process. WindowControl.SetTitle already went through
// runOnMain; this one did not. Off the main thread the call must be queued
// for the next OnUpdate, which a window with no App can still show: the
// queue grows by one and nothing is touched.
func TestSetTitleOffTheMainThreadIsQueued(t *testing.T) {
	w := &window{}
	ran := make(chan bool)
	go func() {
		// Pinned to whichever thread this goroutine is on; almost never the
		// main one, and if it is, the test says so rather than guessing.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if objc.Init() == nil && objc.Class("NSThread").SendBool("isMainThread") {
			ran <- false
			return
		}
		w.SetTitle("queued")
		ran <- true
	}()
	if !<-ran {
		t.Skip("the test goroutine landed on the main thread, where SetTitle runs inline; " +
			"this is a scheduling accident, not a defect")
	}
	w.mainMu.Lock()
	n := len(w.mainQ)
	w.mainMu.Unlock()
	if n != 1 {
		t.Errorf("SetTitle off the main thread queued %d task(s), want 1", n)
	}
}
