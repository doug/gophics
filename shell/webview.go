package shell

import "github.com/doug/gophics/geom"

// WebView capability: embed external web content — an OAuth flow, a map, a rich
// document, a video — as a native subview layered over the gophics surface. A
// Window exposes it by implementing WebViewWindow; widgets reach it via
// ctx.WebView(), nil where unsupported.
//
// This is the one capability that deliberately breaks the "gophics draws every
// pixel" model: the content is a real platform view (an <iframe> on web,
// WKWebView / Android WebView / WebView2 on native), positioned in gophics
// coordinates but composited by the OS. It is NOT part of the scene, damage, or
// screenshots, and it renders above all gophics content. Use it as an escape
// hatch, not a building block.
//
// STATUS: the web implementation (a positioned <iframe> overlay) is complete.
// The native implementations are the WebView-per-OS piece; see
// internal/capgen/README.md.

// WebViewWindow is implemented by a Window that can host an embedded web view.
type WebViewWindow interface {
	WebView() WebView
}

// WebView opens embedded web content.
type WebView interface {
	// Open loads url in a subview at bounds (surface pixels) and returns a
	// handle to move or close it.
	Open(url string, bounds geom.Rect) WebViewHandle
}

// WebViewHandle controls a live web view.
type WebViewHandle interface {
	SetBounds(geom.Rect)
	Close()
}
