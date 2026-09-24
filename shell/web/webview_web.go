//go:build js && wasm

// Web implementation of the shell webview capability (shell/webview.go): a
// positioned <iframe> layered over the canvas. It is a real DOM element
// composited by the browser above the gophics surface — not part of the scene.

package web

import (
	"strconv"
	"syscall/js"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func (w *window) WebView() shell.WebView { return &webWebView{win: w} }

type webWebView struct{ win *window }

func (v *webWebView) Open(url string, bounds geom.Rect) shell.WebViewHandle {
	f := v.win.doc.Call("createElement", "iframe")
	f.Set("src", url)
	f.Get("style").Set("position", "fixed")
	f.Get("style").Set("border", "0")
	f.Get("style").Set("zIndex", "1000")
	h := &webWebViewHandle{f: f, win: v.win}
	h.SetBounds(bounds)
	v.win.doc.Get("body").Call("appendChild", f)
	return h
}

type webWebViewHandle struct {
	f   js.Value
	win *window
}

func cssPx(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }

// SetBounds places the iframe over the given logical rect of the canvas. The
// bounds are mapped through the canvas's on-screen box (canvasBox): the
// canvas is offset by the host page's top inset and the letterbox margins
// and scaled under ScaleToFit, so raw logical pixels put the frame off the
// content it belongs to, and at the wrong size.
func (h *webWebViewHandle) SetBounds(b geom.Rect) {
	left, top, fit := h.win.canvasBox()
	s := h.f.Get("style")
	s.Set("left", cssPx(left+float64(b.Min.X)*fit))
	s.Set("top", cssPx(top+float64(b.Min.Y)*fit))
	s.Set("width", cssPx(float64(b.Dx())*fit))
	s.Set("height", cssPx(float64(b.Dy())*fit))
}

func (h *webWebViewHandle) Close() {
	if p := h.f.Get("parentNode"); !p.IsNull() {
		p.Call("removeChild", h.f)
	}
}
