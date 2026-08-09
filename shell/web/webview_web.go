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

func (w *window) WebView() shell.WebView { return &webWebView{doc: w.doc} }

type webWebView struct{ doc js.Value }

func (v *webWebView) Open(url string, bounds geom.Rect) shell.WebViewHandle {
	f := v.doc.Call("createElement", "iframe")
	f.Set("src", url)
	f.Get("style").Set("position", "fixed")
	f.Get("style").Set("border", "0")
	f.Get("style").Set("zIndex", "1000")
	h := &webWebViewHandle{f: f}
	h.SetBounds(bounds)
	v.doc.Get("body").Call("appendChild", f)
	return h
}

type webWebViewHandle struct{ f js.Value }

func cssPx(v float32) string { return strconv.FormatFloat(float64(v), 'f', 0, 32) + "px" }

func (h *webWebViewHandle) SetBounds(b geom.Rect) {
	s := h.f.Get("style")
	s.Set("left", cssPx(b.Min.X))
	s.Set("top", cssPx(b.Min.Y))
	s.Set("width", cssPx(b.Dx()))
	s.Set("height", cssPx(b.Dy()))
}

func (h *webWebViewHandle) Close() {
	if p := h.f.Get("parentNode"); !p.IsNull() {
		p.Call("removeChild", h.f)
	}
}
