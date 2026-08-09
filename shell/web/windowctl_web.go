//go:build js && wasm

// Web implementation of the shell window-control capability
// (shell/windowctl.go). It maps onto the browser's document/Fullscreen APIs:
// title -> document.title, fullscreen -> the Fullscreen API on the document
// element, size -> the canvas's logical size (the full viewport). The browser
// owns the actual window, so these are best-effort within what a page may do.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// WindowControl satisfies shell.WindowControlWindow for the web shell.
func (w *window) WindowControl() shell.WindowControl { return &webWindowControl{w: w} }

type webWindowControl struct{ w *window }

// SetTitle sets document.title.
func (c *webWindowControl) SetTitle(title string) { c.w.doc.Set("title", title) }

// SetFullscreen requests or exits fullscreen on the document element. Entering
// must be called from a user gesture or the browser rejects the request; the
// returned promise is intentionally ignored (fire-and-forget, matching the
// synchronous capability contract).
func (c *webWindowControl) SetFullscreen(on bool) {
	if on {
		el := c.w.doc.Get("documentElement")
		if fn := el.Get("requestFullscreen"); fn.Type() == js.TypeFunction {
			el.Call("requestFullscreen")
		}
		return
	}
	if fn := c.w.doc.Get("exitFullscreen"); fn.Type() == js.TypeFunction {
		c.w.doc.Call("exitFullscreen")
	}
}

// Fullscreen reports whether document.fullscreenElement is set.
func (c *webWindowControl) Fullscreen() bool {
	return !c.w.doc.Get("fullscreenElement").IsNull()
}

// Size returns the canvas logical size (the viewport). The browser owns the tab
// dimensions, so this is read-only on web; there is no SetSize in the
// capability (see shell/windowctl.go).
func (c *webWindowControl) Size() (w, h float32) {
	return c.w.logical.W, c.w.logical.H
}
