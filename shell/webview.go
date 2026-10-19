package shell

import "github.com/doug/gophics/geom"

// WebView capability: embed external web content — an OAuth flow, a map, a
// rich document, a video — as a native subview layered over the gophics
// surface. A Window exposes it by implementing WebViewWindow; widgets reach it
// via ctx.WebView(), nil where unsupported.
//
// This is the narrow, older face of PlatformViews (shell/platformview.go): a
// web view is a *kind* of platform view, so every shell implements the
// registry and derives this from it with WebViewOf. Read the doc comment there
// for the model and, in particular, for the two things that are permanently
// impossible — a platform view cannot be interleaved between widget layers,
// and it never appears in a snapshot, so a golden test of a screen containing
// one shows a hole. Both apply here unchanged.
//
// Prefer widget.PlatformView over calling this: it owns the handle for its
// element's lifetime and pushes bounds from the layout pass, where this
// interface leaves both to the caller. A handle opened here and not closed
// outlives whatever opened it.

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
