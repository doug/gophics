//go:build js && wasm

// Web implementation of the shell platform-view capability
// (shell/platformview.go). The browser is the one platform where this was
// always easy: a DOM element is a sibling of the <canvas>, the browser
// composites it above, and there is no handle to go looking for. Everything
// here is positioning.
//
// One kind is offered, "webview" — an <iframe>. A map or a video *could* be an
// <iframe> or a <video> element and are deliberately not: the registry's
// contract is that a listed kind works, and shipping "map" on web as an iframe
// pointing at nothing would make Kinds() a promise the shell cannot keep. A
// host-specific build that wants more kinds is the extension point's job, not
// this file's.

package web

import (
	"strconv"
	"syscall/js"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func (w *window) PlatformViews() shell.PlatformViews { return webPlatformViews{win: w} }

type webPlatformViews struct{ win *window }

func (v webPlatformViews) Kinds() []string { return []string{shell.PlatformViewWebView} }

func (v webPlatformViews) Open(kind, params string, bounds geom.Rect, h shell.PlatformViewHandlers) shell.PlatformViewHandle {
	if kind != shell.PlatformViewWebView {
		// Not a kind this shell listed. Returning nil rather than an inert
		// handle keeps the "a listed kind works" contract readable from the
		// call site: a nil handle is a programming error, an OnError is a
		// runtime one.
		return nil
	}
	f := v.win.doc.Call("createElement", "iframe")
	f.Set("src", params)
	f.Get("style").Set("position", "fixed")
	f.Get("style").Set("border", "0")
	// Above the canvas, which is the whole model: the browser composites this
	// over everything gophics draws and nothing can be drawn over it.
	f.Get("style").Set("zIndex", "1000")
	hdl := &webPlatformViewHandle{f: f, win: v.win, h: h}
	hdl.SetBounds(bounds)
	v.win.doc.Get("body").Call("appendChild", f)

	// load/error are the browser's OnReady/OnError. They are registered before
	// the element is in the document but after src is set, which is the only
	// ordering that cannot miss a cached frame's load event.
	if h.OnReady != nil {
		hdl.onload = js.FuncOf(func(js.Value, []js.Value) any { h.OnReady(); return nil })
		f.Call("addEventListener", "load", hdl.onload)
	}
	if h.OnError != nil {
		hdl.onerror = js.FuncOf(func(js.Value, []js.Value) any {
			h.OnError(iframeLoadFailed)
			return nil
		})
		f.Call("addEventListener", "error", hdl.onerror)
	}
	// A message from the embedded page arrives on the *window*, not on the
	// element, so the listener has to filter by source or every iframe on the
	// page would hear every other one. An uncooperative page never posts and
	// the listener simply never fires, which is the honest outcome: OnMessage
	// is what a host-controlled document can use, not a channel to arbitrary
	// web content.
	if h.OnMessage != nil {
		hdl.onmsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) == 0 {
				return nil
			}
			e := args[0]
			if !e.Get("source").Equal(f.Get("contentWindow")) {
				return nil
			}
			h.OnMessage(e.Get("data").String())
			return nil
		})
		js.Global().Call("addEventListener", "message", hdl.onmsg)
		hdl.msgListening = true
	}
	return hdl
}

// iframeLoadFailed is a value rather than a fresh error per failure because
// the browser tells us nothing about why: cross-origin load errors are not
// reported to the embedder at all, and the "error" event that does fire
// carries no detail. Pretending to a reason would be worse than one honest
// sentence.
var iframeLoadFailed = errString("the embedded page could not be loaded; the browser does not say why")

type errString string

func (e errString) Error() string { return string(e) }

type webPlatformViewHandle struct {
	f               js.Value
	win             *window
	h               shell.PlatformViewHandlers
	onload, onerror js.Func
	onmsg           js.Func
	msgListening    bool
	closed          bool
}

func cssPx(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }

// SetBounds places the iframe over the given logical rect of the canvas. The
// bounds are mapped through the canvas's on-screen box (canvasBox): the canvas
// is offset by the host page's top inset and the letterbox margins and scaled
// under ScaleToFit, so raw logical pixels put the frame off the content it
// belongs to, and at the wrong size.
//
// An empty rect hides the element rather than collapsing it to 0×0. A 0×0
// iframe is still loaded, still runs script and still takes focus on tab; a
// display:none one keeps its document and its state — a half-filled form, a
// scroll position — which is what "scrolled out of the viewport" has to mean
// for the capability's promise that state survives.
func (h *webPlatformViewHandle) SetBounds(b geom.Rect) {
	if h.closed {
		return
	}
	s := h.f.Get("style")
	if b.IsEmpty() {
		s.Set("display", "none")
		return
	}
	s.Set("display", "block")
	left, top, fit := h.win.canvasBox()
	s.Set("left", cssPx(left+float64(b.Min.X)*fit))
	s.Set("top", cssPx(top+float64(b.Min.Y)*fit))
	s.Set("width", cssPx(float64(b.Dx())*fit))
	s.Set("height", cssPx(float64(b.Dy())*fit))
}

// Send navigates the frame to msg, the one thing an embedder may do to a
// cross-origin iframe. postMessage would be the richer answer and is
// unavailable in general: it requires the embedded document to cooperate, and
// a page the app does not control will not.
func (h *webPlatformViewHandle) Send(msg string) {
	if h.closed || msg == "" {
		return
	}
	h.f.Set("src", msg)
}

func (h *webPlatformViewHandle) Close() {
	if h.closed {
		return
	}
	h.closed = true
	if !h.onload.IsUndefined() {
		h.f.Call("removeEventListener", "load", h.onload)
		h.onload.Release()
	}
	if !h.onerror.IsUndefined() {
		h.f.Call("removeEventListener", "error", h.onerror)
		h.onerror.Release()
	}
	if h.msgListening {
		js.Global().Call("removeEventListener", "message", h.onmsg)
		h.onmsg.Release()
	}
	if p := h.f.Get("parentNode"); !p.IsNull() {
		p.Call("removeChild", h.f)
	}
}

// Both faces of the capability, checked at compile time. shell/coverage_test.go
// finds the accessor by scanning source, which cannot tell whether it actually
// satisfies the interface; these can.
var (
	_ shell.PlatformViewWindow = (*window)(nil)
	_ shell.WebViewWindow      = (*window)(nil)
)
