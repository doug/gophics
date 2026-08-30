//go:build !js

// Desktop implementation of the shell window-control capability
// (shell/windowctl.go). A pass-through to the gogpu App, which owns the
// platform window — but every mutating call goes through runOnMain first.
//
// That is not optional on macOS. gogpu runs OnUpdate and input on the main
// thread and OnDraw on a render thread, and gophics drives the widget tree from
// OnDraw, so a tap handler is *not* on the main thread. AppKit answers a window
// mutation from anywhere else with an uncatchable Objective-C exception:
//
//	'NSInternalInconsistencyException', reason: 'NSWindow geometry should only
//	be modified on the main thread!'
//
// which aborts the process. Both SetTitle and SetFullscreen did exactly that —
// clicking "Toggle fullscreen" in examples/capabilities killed the app, and the
// two calls crash independently, so this was never about fullscreen. See
// mainthread.go, whose own comment predicted it: "a capability that touched
// AppKit from Build would abort the process".
//
// The header here used to say the file was "build-verified on darwin" and that
// the behaviour was "not runtime-confirmed". It was not runtime-confirmed
// because it did not work.
//
// KNOWN ISSUE, separate from the crash and not fixed here: entering fullscreen
// stops the UI loop. After SetFullscreen(true) no further frames are produced,
// so the app freezes on screen and cannot be taken back out programmatically —
// a posted callback that would call SetFullscreen(false) never runs. Verified
// against a control that posts on the same schedule without touching the
// window, which runs to completion. The defect is in the gogpu render loop
// rather than in this binding.

package desktop

import "github.com/doug/gophics/shell"

// WindowControl satisfies shell.WindowControlWindow for the desktop shell.
func (w *window) WindowControl() shell.WindowControl { return desktopWindowControl{w: w} }

type desktopWindowControl struct{ w *window }

// SetTitle forwards to gogpu App.SetTitle on the main thread.
func (c desktopWindowControl) SetTitle(title string) {
	c.w.runOnMain(func() { c.w.app.SetTitle(title) })
}

// SetFullscreen forwards to gogpu App.SetFullscreen (native toggleFullScreen on
// macOS, borderless on Windows, EWMH/xdg on Linux) on the main thread.
func (c desktopWindowControl) SetFullscreen(on bool) {
	c.w.runOnMain(func() { c.w.app.SetFullscreen(on) })
}

// Fullscreen forwards to gogpu App.IsFullscreen.
//
// Not wrapped, because it has to answer now and runOnMain queues when the
// caller is off the main thread. It reads NSWindow's style mask, which is a
// property read rather than a geometry mutation and does not raise the
// exception the setters do. The window cannot enter or leave fullscreen between
// this read and the queued toggle that usually follows it, because that
// transition is itself main-thread work that cannot run until this returns.
func (c desktopWindowControl) Fullscreen() bool { return c.w.app.IsFullscreen() }

// Size returns the logical window size from gogpu App.Size. A read, like
// Fullscreen above.
func (c desktopWindowControl) Size() (w, h float32) {
	lw, lh := c.w.app.Size()
	return float32(lw), float32(lh)
}

// SetSize resizes the window, reporting whether the platform allowed it.
//
// This used to be deliberately absent, on the grounds that gogpu exposed no
// runtime resize. It does now: PlatformWindow.SetSize is implemented by
// AppKit's setFrame on macOS, SetWindowPos on Windows and an X11 configure
// request, and declined by Wayland, where the compositor owns geometry.
// The bool means the request was accepted, which is as strong as the threading
// allows. Everywhere except macOS the work runs inline and this is the
// platform's own answer — including Wayland's refusal, where the compositor
// owns geometry. On macOS it is queued for the main thread and the outcome is
// not knowable at return, so setSizeOnMain answers for AppKit, which accepts
// programmatic resize.
//
// Capturing the queued result into a variable and returning it would be a data
// race: the render thread returns before the main thread writes.
func (c desktopWindowControl) SetSize(w, h float32) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	return c.w.setSizeOnMain(int(w), int(h))
}
