package shell_test

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// WebViewOf is the whole of the "a web view is a kind, not a capability"
// decision expressed as code: every shell implements the registry once and the
// older, narrower capability falls out of it. What has to hold is that it stays
// an honest nil — the invariant in AGENTS.md is that a capability which always
// returns something is worse than one that returns nothing, because a caller
// cannot then tell "no web view here" from "this build cannot embed one".

type stubViews struct {
	kinds  []string
	opened []string
}

func (s *stubViews) Kinds() []string { return s.kinds }

func (s *stubViews) Open(kind, params string, _ geom.Rect, _ shell.PlatformViewHandlers) shell.PlatformViewHandle {
	s.opened = append(s.opened, kind+" "+params)
	return stubHandle{}
}

type stubHandle struct{}

func (stubHandle) SetBounds(geom.Rect) {}
func (stubHandle) Send(string)         {}
func (stubHandle) Close()              {}

func TestWebViewOfIsNilWithoutTheWebViewKind(t *testing.T) {
	if got := shell.WebViewOf(nil); got != nil {
		t.Errorf("WebViewOf(nil) = %v, want nil", got)
	}
	// A host that can embed views but not a browser: the registry is present
	// and WebView must still be nil, or an app shows an in-app-browser button
	// that opens nothing.
	if got := shell.WebViewOf(&stubViews{kinds: []string{shell.PlatformViewMap}}); got != nil {
		t.Errorf("WebViewOf(map-only) = %v, want nil", got)
	}
	// And an empty registry — a host that registered and offers nothing — is
	// the third real configuration, distinguishable from nil to an app that
	// reads Kinds but not to one that only wants a web view.
	if got := shell.WebViewOf(&stubViews{}); got != nil {
		t.Errorf("WebViewOf(empty) = %v, want nil", got)
	}
}

func TestWebViewOfOpensTheWebViewKind(t *testing.T) {
	pv := &stubViews{kinds: []string{shell.PlatformViewWebView, shell.PlatformViewMap}}
	wv := shell.WebViewOf(pv)
	if wv == nil {
		t.Fatal("WebViewOf returned nil for a registry offering the webview kind")
	}
	h := wv.Open("https://example.com", geom.RectXYWH(0, 0, 10, 10))
	if h == nil {
		t.Fatal("Open returned no handle")
	}
	if len(pv.opened) != 1 || pv.opened[0] != "webview https://example.com" {
		t.Errorf("opened %v; the URL must arrive as the kind's params", pv.opened)
	}
}

// The handle types have to stay assignable, because that is what lets the
// derivation be free rather than another wrapper: a PlatformViewHandle is a
// WebViewHandle with one extra method.
func TestPlatformViewHandleSatisfiesWebViewHandle(t *testing.T) {
	var _ shell.WebViewHandle = shell.PlatformViewHandle(stubHandle{})
}
