//go:build darwin && !ios && !js

package desktop

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

func rect(x, y, w, h float32) geom.Rect {
	return geom.Rect{Min: geom.Pt{X: x, Y: y}, Max: geom.Pt{X: x + w, Y: y + h}}
}

// The Y flip is the one piece of arithmetic in this backend that no compiler
// and no interface check can catch, and getting it wrong puts a web view at
// the mirror image of where the layout said — which looks like a layout bug
// rather than a coordinate-space bug. It is also the only part that can be
// tested without a window on screen.
func TestMacFrameFlipsToBottomLeftOrigin(t *testing.T) {
	const contentH = 600

	for _, tc := range []struct {
		name string
		in   geom.Rect
		want cgRect
	}{
		{"top-left corner sits at the top of the content view",
			rect(0, 0, 100, 40), cgRect{x: 0, y: 560, w: 100, h: 40}},
		{"bottom-left corner sits at the origin",
			rect(0, 560, 100, 40), cgRect{x: 0, y: 0, w: 100, h: 40}},
		{"full bleed keeps its height",
			rect(0, 0, 800, 600), cgRect{x: 0, y: 0, w: 800, h: 600}},
		{"an inset rect keeps its inset from the *bottom*",
			rect(20, 100, 200, 220), cgRect{x: 20, y: 280, w: 200, h: 220}},
	} {
		if got := macFrame(tc.in, contentH); got != tc.want {
			t.Errorf("%s: macFrame(%v, %v) = %+v, want %+v", tc.name, tc.in, contentH, got, tc.want)
		}
	}
}

// The scale is deliberately absent: AppKit frames are in points and gophics
// logical pixels are points on macOS, so the conversion the mobile backend has
// to do would be a double-scaling here. This pins that down, because "it works
// at 1x" is exactly how the opposite mistake survives.
func TestMacFrameDoesNotApplyDeviceScale(t *testing.T) {
	got := macFrame(rect(10, 10, 100, 100), 400)
	if got.x != 10 || got.w != 100 || got.h != 100 {
		t.Fatalf("macFrame scaled a logical rect: %+v", got)
	}
	if got.y != 290 {
		t.Fatalf("flip used the wrong height: y = %v, want 290", got.y)
	}
}

func TestKindsOffersWebViewOnly(t *testing.T) {
	kinds := macPlatformViews{}.Kinds()
	if len(kinds) != 1 || kinds[0] != shell.PlatformViewWebView {
		t.Fatalf("Kinds() = %v, want just %q", kinds, shell.PlatformViewWebView)
	}
}

// A kind this shell did not list must come back nil, not as a handle that
// later reports a failure: widget.PlatformView treats a nil handle as
// ErrPlatformViewKind and stops retrying, and that latch is what keeps a
// mistyped kind from reopening on every frame.
func TestOpenRefusesAnUnlistedKind(t *testing.T) {
	p := macPlatformViews{w: &window{}}
	for _, kind := range []string{shell.PlatformViewMap, shell.PlatformViewVideo, "", "WebView"} {
		if h := p.Open(kind, "x", rect(0, 0, 10, 10), shell.PlatformViewHandlers{}); h != nil {
			t.Errorf("Open(%q) returned a handle; want nil", kind)
		}
	}
}

// Close is idempotent and safe before the view exists, and SetBounds and Send
// are safe after it. widget.PlatformView calls Close from Dispose without
// knowing whether creation ever ran, so every one of these is a path the
// framework actually takes — a view opened and unmounted in the same frame
// never reaches the main thread at all.
func TestHandleIsSafeBeforeCreationAndAfterClose(t *testing.T) {
	var errs []error
	v := &macPlatformView{
		w:        &window{},
		handlers: shell.PlatformViewHandlers{OnError: func(err error) { errs = append(errs, err) }},
	}

	v.SetBounds(rect(0, 0, 100, 100))
	v.Send("https://example.com")
	v.Close()
	v.Close()
	v.SetBounds(rect(0, 0, 50, 50))
	v.Send("https://example.com")

	if v.view != 0 {
		t.Errorf("a closed handle still holds a view: %v", v.view)
	}
	if len(errs) != 0 {
		t.Errorf("handle reported %d errors with no view to fail: %v", len(errs), errs)
	}

	// create must decline to build anything for a handle that is already
	// closed. Without that check an Open immediately followed by a Dispose
	// would leave a WKWebView in the window with nothing left holding a
	// reference to close it.
	v.create("https://example.com")
	if v.view != 0 {
		t.Errorf("create built a view for a closed handle: %v", v.view)
	}
}

// Exactly one of OnReady/OnError fires per opened view, and WKWebView reports
// every navigation — including ones the page starts itself — so the latching
// is load-bearing rather than defensive.
func TestReadyAndErrorFireOnceAndExcludeEachOther(t *testing.T) {
	t.Run("ready wins over a later failure", func(t *testing.T) {
		var ready, failed int
		v := &macPlatformView{handlers: shell.PlatformViewHandlers{
			OnReady: func() { ready++ },
			OnError: func(error) { failed++ },
		}}
		v.markReady()
		v.markReady()
		v.fail(errNope)
		if ready != 1 || failed != 0 {
			t.Fatalf("ready=%d failed=%d, want 1 and 0", ready, failed)
		}
	})
	t.Run("a failure is reported once and blocks a later ready", func(t *testing.T) {
		var ready, failed int
		v := &macPlatformView{handlers: shell.PlatformViewHandlers{
			OnReady: func() { ready++ },
			OnError: func(error) { failed++ },
		}}
		v.fail(errNope)
		v.fail(errNope)
		v.markReady()
		if ready != 0 || failed != 1 {
			t.Fatalf("ready=%d failed=%d, want 0 and 1", ready, failed)
		}
	})
}

var errNope = macError(0)

// macError must never return nil, because fail latches on having reported
// something and a nil error would travel all the way to an app's OnError.
func TestMacErrorIsNeverNil(t *testing.T) {
	if err := macError(0); err == nil || err.Error() == "" {
		t.Fatalf("macError(nil) = %v", err)
	}
}

// The registry is keyed by the WKWebView pointer because that is what AppKit
// hands the delegate; a view that has been forgotten must not resolve, or a
// navigation completing during teardown would call into a closed handle.
func TestViewRegistryForgets(t *testing.T) {
	v := &macPlatformView{}
	const id = objc.ID(0xBEEF)
	macViews.register(id, v)
	if macViews.find(id) != v {
		t.Fatal("register did not make the view findable")
	}
	macViews.forget(id)
	if macViews.find(id) != nil {
		t.Fatal("forget left the view findable")
	}
}

// sendRect is the one FFI call shape this file adds to internal/objc, and the
// thing that can be wrong about it — the CGRect reaching the callee as four
// doubles in the right order — is invisible from Go. CALayer is used as the
// witness because Core Animation supports KVC key paths into a CGRect
// property, so the frame can be read back through scalar-returning messages
// alone; AppKit's own -frame would need a struct *return*, which this file
// deliberately does not implement.
func TestSendRectPassesACGRectByValue(t *testing.T) {
	if err := objc.LoadFramework("QuartzCore"); err != nil {
		t.Skipf("capability, not defect: QuartzCore did not load: %v", err)
	}
	cls := objc.Class("CALayer")
	if !cls.Valid() {
		t.Skip("capability, not defect: CALayer is not registered in this process")
	}
	layer := cls.Send("layer")
	if !layer.Valid() {
		t.Skip("capability, not defect: could not create a CALayer")
	}

	want := cgRect{x: 12, y: 34, w: 56, h: 78}
	sendRect(layer, "setFrame:", want)

	read := func(path string) float64 {
		n := layer.Send("valueForKeyPath:", objc.Obj(objc.String(path)))
		if !n.Valid() {
			t.Fatalf("CALayer did not answer %q", path)
		}
		return n.SendDouble("doubleValue")
	}
	got := cgRect{
		x: read("frame.origin.x"),
		y: read("frame.origin.y"),
		w: read("frame.size.width"),
		h: read("frame.size.height"),
	}
	if got != want {
		t.Fatalf("setFrame: round-tripped %+v as %+v — the CGRect argument is "+
			"not reaching the callee intact", want, got)
	}
}

// sendRect must not crash or claim success on a nil receiver, a selector that
// does not exist, or more trailing arguments than it can box. Open's failure
// paths run these: a window that has gone away leaves nil objects behind.
func TestSendRectRejectsBadCalls(t *testing.T) {
	if objc.Init() != nil {
		t.Skip("capability, not defect: the Objective-C runtime did not load")
	}
	if got := sendRect(0, "setFrame:", cgRect{}); got != 0 {
		t.Errorf("sendRect on a nil receiver returned %v", got)
	}
	cls := objc.Class("NSObject")
	if cls.Valid() {
		if got := sendRect(cls, "setFrame:", cgRect{}, 1, 2, 3); got != 0 {
			t.Errorf("sendRect with too many trailing args returned %v", got)
		}
	}
}

// The delegate class carries the four selectors WebKit calls, and it is
// defined once for the process. Checking it registers at all is worth a test
// because every failure mode inside DefineClass is silent by design — a
// delegate that did not register leaves OnReady and OnError never firing,
// which looks exactly like a page that is still loading.
func TestPlatformViewDelegateRegisters(t *testing.T) {
	if objc.Init() != nil {
		t.Skip("capability, not defect: the Objective-C runtime did not load")
	}
	d := platformViewDelegate()
	if !d.Valid() {
		t.Fatal("platformViewDelegate() did not produce an object")
	}
	if second := platformViewDelegate(); second != d {
		t.Fatalf("platformViewDelegate() built a second delegate (%v then %v); "+
			"Objective-C class names are global and the class is never "+
			"unregistered, so this must be one object", d, second)
	}
	for _, sel := range []string{
		"webView:didFinishNavigation:",
		"webView:didFailNavigation:withError:",
		"webView:didFailProvisionalNavigation:withError:",
		"userContentController:didReceiveScriptMessage:",
	} {
		if !d.SendBool("respondsToSelector:", objc.SelArg(objc.Sel(sel))) {
			t.Errorf("the delegate does not respond to %s; WebKit dispatches "+
				"through -respondsToSelector:, so this callback would never arrive", sel)
		}
	}
}
