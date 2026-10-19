//go:build js && wasm

// The web shell's WebView capability is derived from its platform-view
// registry (platformview_web.go) rather than implemented twice. A web view is
// a kind of platform view, and there is exactly one <iframe> story on this
// platform; two entry points to it would drift.
//
// shell.WebViewOf returns nil unless the registry offers the "webview" kind,
// so ctx.WebView() stays an honest nil on a shell that embeds nothing instead
// of a handle to nothing.

package web

import "github.com/doug/gophics/shell"

func (w *window) WebView() shell.WebView { return shell.WebViewOf(w.PlatformViews()) }
