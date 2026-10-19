//go:build darwin && !ios && !js

// macOS implementation of the platform-view capability
// (shell/platformview.go): a real WKWebView, added as a subview of the window
// gophics is drawing into, positioned from the layout pass. Driven through the
// pure-Go Objective-C bridge in internal/objc — no CGo, the same mechanism the
// macOS FilePicker uses for NSOpenPanel.
//
// # Finding the window, and why that is not a heuristic
//
// gogpu owns the NSWindow and does not hand it out: the handle lives behind
// two unexported struct fields and an internal/ boundary. What it does expose
// is Window.NativeHandles, which on macOS answers (0, CAMetalLayer*) — the
// layer gophics renders into. That is not a view, so it cannot be a parent,
// but it is a unique identity: gogpu installs it with
// -[NSView setLayer:] on the content view (layer-hosting mode), so the one
// NSWindow whose contentView.layer is that pointer is this window.
//
// So the window is found by asking AppKit for its own windows —
// [[NSApplication sharedApplication] windows] — and matching on the layer.
// That is an exact identification rather than "probably the first window",
// which matters as soon as an app opens a second window or a panel: a
// title-or-position match would be a guess, and would put a web view in the
// wrong window rather than failing.
//
// # Coordinates: the 1x/2x trap runs the other way here
//
// A native subview is positioned in the units its superview uses, and on macOS
// that is *points*, not device pixels. gogpu's logical size is the content
// view's bounds in points (App.Size), and gophics logical pixels are exactly
// that, so bounds cross this boundary **unscaled**. Multiplying by the device
// scale — the thing the mobile backend has to do, because Android view
// geometry is in pixels — would place every view at twice its offset and twice
// its size on a Retina display. The scale conversion belongs at the surface,
// and the surface is not here.
//
// What does need converting is the origin: AppKit's default view coordinate
// space is bottom-left, and gophics is top-left. Hence the flip against the
// content view's height on every SetBounds. The height comes from
// gogpu's App.Size rather than from -[NSView bounds] because it is the same
// number the layout pass used, so the flip cannot disagree with the rect it is
// flipping.
//
// # Accessibility: a view placed here is invisible to VoiceOver
//
// Stated plainly because it cannot be fixed from this file. The desktop
// accessibility model is push: the app's semantics are forwarded to the
// windowing layer, which publishes them as a *virtual* tree by calling
// -[NSView setAccessibilityChildren:] on the content view with an array of
// synthesised NSAccessibilityElements (internal/gfx/gogpu/internal/platform/
// darwin/a11y.go). That call replaces the children AppKit would otherwise
// derive from the real subviews, and it is re-made on every semantics change —
// so a WKWebView added here, which carries a perfectly good accessibility
// tree of its own, is cut out of the window's tree the moment gophics
// publishes anything.
//
// Splicing it back in means appending the subview to that array inside the
// vendored fork, in all three desktop backends, and is the larger half of this
// feature rather than a detail of it. Until then: give widget.PlatformView a
// Placeholder with real semantics (a label, a button that calls OpenURL). The
// placeholder *is* in the widget tree, so it is in the published a11y tree,
// and a VoiceOver user at least learns that something is there and has a way
// out. A platform view with no placeholder is, to a screen reader, a hole.
package desktop

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

// PlatformViews publishes the capability; the app runner wires it to the
// widget tree. It is nil when the Objective-C bridge will not load, which is
// the one way this can be genuinely unavailable on a Mac — and nil is what
// lets an app hide the affordance instead of opening a view that never
// appears.
func (w *window) PlatformViews() shell.PlatformViews {
	if objc.Init() != nil {
		return nil
	}
	return macPlatformViews{w: w}
}

// WebView derives the older, narrower capability from the registry rather than
// implementing it twice: a web view is a kind of platform view. See
// shell.WebViewOf.
func (w *window) WebView() shell.WebView { return shell.WebViewOf(w.PlatformViews()) }

type macPlatformViews struct{ w *window }

// Kinds lists only "webview". WebKit ships with every macOS, so that one is a
// promise this shell can keep; "map" and "video" are deliberately absent.
// MKMapView and AVPlayerView are reachable by the same mechanism and are not
// built, and listing a kind that does not open would make Kinds a lie — which
// is the whole reason the registry reports kinds instead of an app guessing.
func (p macPlatformViews) Kinds() []string { return []string{shell.PlatformViewWebView} }

func (p macPlatformViews) Open(kind, params string, bounds geom.Rect, h shell.PlatformViewHandlers) shell.PlatformViewHandle {
	if kind != shell.PlatformViewWebView {
		// Not a kind this shell listed. Nil rather than an inert handle, so a
		// nil handle stays readable at the call site as a programming error
		// and an OnError as a runtime one.
		return nil
	}
	v := &macPlatformView{w: p.w, handlers: h, bounds: bounds}
	// Creation is main-thread work — a WKWebView is an NSView, and AppKit
	// aborts the process rather than erroring when one is built elsewhere —
	// and the caller is usually gogpu's render thread, where Build runs. See
	// mainthread.go. The handle returned here is live immediately regardless:
	// SetBounds and Close record intent under the lock and the queued task
	// reads it, so a view that is closed before it is created is never
	// created.
	p.w.runOnMain(func() { v.create(params) })
	return v
}

// macPlatformView is one live native view.
//
// Everything mutable is behind mu because the two sides arrive on different
// threads: SetBounds and Close come from the widget tree on the render thread,
// while create, the AppKit callbacks and every message send run on the main
// thread. The invariant is that the main-thread tasks only ever *read* intent
// (bounds, closed) and only ever *write* the native fields, so a Close that
// races a create cannot leave a view nobody owns.
type macPlatformView struct {
	w        *window
	handlers shell.PlatformViewHandlers

	mu      sync.Mutex
	bounds  geom.Rect
	view    objc.ID // the WKWebView, 0 until created
	closed  bool
	ready   bool // OnReady has fired; it fires once, not per navigation
	errored bool
}

// create builds the web view and adds it to the window's content view. Main
// thread only.
func (v *macPlatformView) create(params string) {
	v.mu.Lock()
	if v.closed || v.view != 0 {
		v.mu.Unlock()
		return
	}
	v.mu.Unlock()

	parent, err := v.w.macContentView()
	if err != nil {
		v.fail(err)
		return
	}
	if err := objc.LoadFramework("WebKit"); err != nil {
		v.fail(err)
		return
	}
	cls := objc.Class("WKWebView")
	if !cls.Valid() {
		v.fail(errors.New("desktop: WKWebView unavailable"))
		return
	}

	// The configuration has to be supplied at init: -[WKWebView
	// configuration] returns a *copy*, so a script message handler added
	// afterwards would be registered on an object the view does not consult,
	// and OnMessage would simply never fire. That is why this goes through
	// -initWithFrame:configuration: — and why the CGRect-passing helper below
	// exists at all.
	cfg := objc.Class("WKWebViewConfiguration").Send("alloc").Send("init")
	if !cfg.Valid() {
		v.fail(errors.New("desktop: WKWebViewConfiguration unavailable"))
		return
	}
	if ucc := cfg.Send("userContentController"); ucc.Valid() {
		if d := platformViewDelegate(); d.Valid() {
			// The page reaches OnMessage with
			// window.webkit.messageHandlers.gophics.postMessage("…"), which
			// only a document the app controls will do. An uncooperative page
			// never posts and OnMessage never fires, which is the honest
			// outcome rather than a channel to arbitrary web content.
			ucc.SendVoid("addScriptMessageHandler:name:", objc.Obj(d), objc.Obj(objc.String(macMessageHandlerName)))
		}
	}

	view := sendRect(cls.Send("alloc"), "initWithFrame:configuration:", cgRect{}, cfg)
	// The configuration was alloc/init'd here, so this call owns it; the view
	// has copied what it needs by now.
	cfg.SendVoid("release")
	if !view.Valid() {
		v.fail(errors.New("desktop: could not create WKWebView"))
		return
	}
	if d := platformViewDelegate(); d.Valid() {
		view.SendVoid("setNavigationDelegate:", objc.Obj(d))
	}

	// Hidden until the first real rect lands, so a view whose bounds have not
	// been pushed yet never flashes at the window's bottom-left corner — which
	// is where an unset frame puts it in AppKit's coordinate space.
	view.SendVoid("setHidden:", objc.Bool(true))
	parent.SendVoid("addSubview:", objc.Obj(view))

	v.mu.Lock()
	if v.closed {
		// Closed while this task sat in the queue. Tear down here rather than
		// leaving a view with no handle pointing at it, which is exactly the
		// leak widget.PlatformView's Dispose exists to prevent.
		v.mu.Unlock()
		destroyMacView(view)
		return
	}
	v.view = view
	bounds := v.bounds
	v.mu.Unlock()

	macViews.register(view, v)
	v.applyBounds(bounds)

	if params != "" {
		v.navigate(params)
	} else {
		// Nothing to load is not a failure, and nothing will call back, so
		// readiness is reported here: a widget waiting on OnReady for a view
		// that was never asked to load anything would show its placeholder
		// forever.
		v.markReady()
	}
}

// SetBounds records the rect and applies it on the main thread. An empty rect
// hides the view without destroying it, so a web view scrolled out of a list
// keeps its scroll position, its form state and its running page for when it
// scrolls back.
//
// The rect lands one main-thread turn late when the caller is the render
// thread (the common case: this is pushed from paint). On desktop that is a
// frame of lag on a view that is being moved, which is visible only while
// dragging a scroll and is the price of AppKit's thread rule — the alternative
// is touching NSView from the render thread, which aborts the process.
func (v *macPlatformView) SetBounds(r geom.Rect) {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.bounds = r
	have := v.view != 0
	v.mu.Unlock()
	if !have {
		return // create will apply whatever the latest bounds are
	}
	v.w.runOnMain(func() {
		v.mu.Lock()
		cur := v.bounds
		v.mu.Unlock()
		v.applyBounds(cur)
	})
}

// applyBounds maps a gophics rect onto the content view's coordinate space.
// Main thread only.
func (v *macPlatformView) applyBounds(r geom.Rect) {
	v.mu.Lock()
	view, closed := v.view, v.closed
	v.mu.Unlock()
	if view == 0 || closed {
		return
	}
	if r.IsEmpty() {
		view.SendVoid("setHidden:", objc.Bool(true))
		return
	}
	_, h := v.w.app.Size()
	view.SendVoid("setHidden:", objc.Bool(false))
	sendRect(view, "setFrame:", macFrame(r, float64(h)))
}

// macFrame converts a gophics rect (top-left origin, logical points) to an
// AppKit frame (bottom-left origin, points). contentH is the content view's
// height in the same units.
//
// Split out from applyBounds so it can be tested: the flip is the one piece of
// arithmetic here that is wrong in a way no compiler catches and that a Mac
// with a window open is needed to see.
func macFrame(r geom.Rect, contentH float64) cgRect {
	return cgRect{
		x: float64(r.Min.X),
		y: contentH - float64(r.Max.Y),
		w: float64(r.Dx()),
		h: float64(r.Dy()),
	}
}

// Send navigates the view to msg. For the "webview" kind that is the whole of
// what an embedder may usefully say to a page it did not write; richer
// messaging is the page's side of OnMessage, which needs a cooperating
// document.
//
// A Send that arrives before the view exists is dropped rather than queued.
// The only way to hold this handle is through widget.PlatformView's
// Controller, which publishes it after Open returns, so the window where this
// matters is one main-thread turn wide.
func (v *macPlatformView) Send(msg string) {
	if msg == "" {
		return
	}
	v.mu.Lock()
	skip := v.closed || v.view == 0
	v.mu.Unlock()
	if skip {
		return
	}
	v.w.runOnMain(func() { v.navigate(msg) })
}

// navigate loads a URL. Main thread only.
func (v *macPlatformView) navigate(url string) {
	v.mu.Lock()
	view, closed := v.view, v.closed
	v.mu.Unlock()
	if view == 0 || closed {
		return
	}
	nsurl := objc.Class("NSURL").Send("URLWithString:", objc.Obj(objc.String(url)))
	if !nsurl.Valid() {
		v.fail(errors.New("desktop: platform view: not a URL: " + url))
		return
	}
	req := objc.Class("NSURLRequest").Send("requestWithURL:", objc.Obj(nsurl))
	if !req.Valid() {
		v.fail(errors.New("desktop: platform view: could not build a request for " + url))
		return
	}
	view.SendVoid("loadRequest:", objc.Obj(req))
}

// Close destroys the view. Idempotent, and safe from any thread and after a
// failure — widget.PlatformView calls it on unmount without knowing which.
func (v *macPlatformView) Close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	view := v.view
	v.view = 0
	v.mu.Unlock()
	if view == 0 {
		return // create will see closed and tear down, or never ran
	}
	macViews.forget(view)
	v.w.runOnMain(func() { destroyMacView(view) })
}

// destroyMacView unwires and releases one web view. Main thread only.
//
// The delegates go first, so a navigation that completes between here and
// dealloc cannot call back into a handle that is gone. The release balances
// the alloc in create: addSubview: retained it, removeFromSuperview gave that
// back, and this call is the one reference this file ever owned. Leaking it
// instead would leave a page running script and holding sockets with nothing
// left able to stop it, which is the opposite of what Close promises.
func destroyMacView(view objc.ID) {
	if !view.Valid() {
		return
	}
	view.SendVoid("setNavigationDelegate:", objc.Obj(0))
	if cfg := view.Send("configuration"); cfg.Valid() {
		if ucc := cfg.Send("userContentController"); ucc.Valid() {
			ucc.SendVoid("removeScriptMessageHandlerForName:", objc.Obj(objc.String(macMessageHandlerName)))
		}
	}
	view.SendVoid("stopLoading")
	view.SendVoid("removeFromSuperview")
	view.SendVoid("release")
}

// markReady fires OnReady at most once. WKWebView reports every navigation
// finishing, including ones the page starts itself, and the capability's
// contract is exactly one of OnReady/OnError per opened view.
func (v *macPlatformView) markReady() {
	v.mu.Lock()
	if v.closed || v.ready || v.errored {
		v.mu.Unlock()
		return
	}
	v.ready = true
	cb := v.handlers.OnReady
	v.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// fail fires OnError at most once, and not at all once the view has reported
// ready: a page that loads and later fails a sub-navigation has not failed to
// open, and reporting it as such would make a widget tear down a working view.
func (v *macPlatformView) fail(err error) {
	v.mu.Lock()
	if v.closed || v.ready || v.errored {
		v.mu.Unlock()
		return
	}
	v.errored = true
	cb := v.handlers.OnError
	v.mu.Unlock()
	if cb != nil {
		cb(err)
	}
}

func (v *macPlatformView) message(msg string) {
	v.mu.Lock()
	cb := v.handlers.OnMessage
	closed := v.closed
	v.mu.Unlock()
	if cb != nil && !closed {
		cb(msg)
	}
}

// macContentView finds the NSView gophics is drawing into. Main thread only.
//
// The match is on the layer, not on the window: see the package comment. The
// fallback is the key window, used only when the layer identity is not
// available — before the surface exists there is nothing to match against, and
// a nil answer there would make the capability look broken on the first frame.
func (w *window) macContentView() (objc.ID, error) {
	if err := objc.LoadFramework("AppKit"); err != nil {
		return 0, err
	}
	nsapp := objc.Class("NSApplication").Send("sharedApplication")
	if !nsapp.Valid() {
		return 0, errors.New("desktop: no NSApplication")
	}
	windows := objc.Array(nsapp.Send("windows"))
	if len(windows) == 0 {
		return 0, errors.New("desktop: the application has no windows yet")
	}

	var layer uintptr
	if pw := w.app.PrimaryWindow(); pw != nil {
		_, layer = pw.NativeHandles()
	}
	if layer != 0 {
		for _, win := range windows {
			cv := win.Send("contentView")
			if !cv.Valid() {
				continue
			}
			if uintptr(cv.Send("layer")) == layer {
				return cv, nil
			}
		}
	}
	// No identity to match, or no window carries it. The key window is the
	// best remaining answer and is right for the single-window case this
	// reaches, but it is a guess, so say so rather than silently parenting
	// into whichever window happened to be first.
	for _, sel := range []string{"keyWindow", "mainWindow"} {
		if win := nsapp.Send(sel); win.Valid() {
			if cv := win.Send("contentView"); cv.Valid() {
				return cv, nil
			}
		}
	}
	return 0, errors.New("desktop: could not find the window gophics is drawing into")
}

// --- the shared delegate -----------------------------------------------------

// macMessageHandlerName is the name a page posts to:
// window.webkit.messageHandlers.gophics.postMessage("…").
const macMessageHandlerName = "gophics"

// One delegate object serves every view in the process, with the view pointer
// as the key. A delegate per view would mean a runtime-defined class per view
// (Objective-C class names are global and a registered class is never
// unregistered), so the lookup goes the other way: AppKit hands the callback
// the WKWebView, and that finds the handle.
var macViews = &macViewRegistry{byView: map[objc.ID]*macPlatformView{}}

// The lock is real rather than defensive. AppKit delivers these callbacks on
// the main thread while Open and Close run on whichever goroutine the widget
// tree is on, so register and find genuinely race.
type macViewRegistry struct {
	mu     sync.Mutex
	byView map[objc.ID]*macPlatformView
}

func (r *macViewRegistry) register(view objc.ID, v *macPlatformView) {
	r.mu.Lock()
	r.byView[view] = v
	r.mu.Unlock()
}

func (r *macViewRegistry) forget(view objc.ID) {
	r.mu.Lock()
	delete(r.byView, view)
	r.mu.Unlock()
}

func (r *macViewRegistry) find(view objc.ID) *macPlatformView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byView[view]
}

var (
	delegateOnce sync.Once
	delegateObj  objc.ID
)

// platformViewDelegate returns the process-wide delegate, defining its class
// on first use.
//
// The class declares no protocol conformance and does not need to: AppKit
// dispatches delegate messages through -respondsToSelector:, so implementing
// the selectors is the whole contract. The encodings are the runtime's
// description of each signature and are not checked — "v@:@@" is a void
// method taking self, _cmd and two objects — so they and the Go signatures
// have to be kept in step by hand.
func platformViewDelegate() objc.ID {
	delegateOnce.Do(func() {
		def, err := objc.DefineClass("GophicsPlatformViewDelegate", "NSObject")
		if err != nil {
			return
		}
		methods := []struct {
			sel, enc string
			fn       any
		}{
			{"webView:didFinishNavigation:", "v@:@@", func(_, _, webView, _ uintptr) {
				if v := macViews.find(objc.ID(webView)); v != nil {
					v.markReady()
				}
			}},
			{"webView:didFailNavigation:withError:", "v@:@@@", func(_, _, webView, _, err uintptr) {
				if v := macViews.find(objc.ID(webView)); v != nil {
					v.fail(macError(objc.ID(err)))
				}
			}},
			// A bad host, a refused TLS handshake and a scheme nothing handles
			// all fail *provisionally* — before the navigation commits — and
			// arrive only on this selector. Implementing just the committed
			// one would leave the most common failure silent, which is the
			// shape the capability's doc comment calls worse than an error: a
			// widget waiting on a view that will never arrive.
			{"webView:didFailProvisionalNavigation:withError:", "v@:@@@", func(_, _, webView, _, err uintptr) {
				if v := macViews.find(objc.ID(webView)); v != nil {
					v.fail(macError(objc.ID(err)))
				}
			}},
			{"userContentController:didReceiveScriptMessage:", "v@:@@", func(_, _, _, msg uintptr) {
				m := objc.ID(msg)
				if !m.Valid() {
					return
				}
				if v := macViews.find(m.Send("webView")); v != nil {
					v.message(objc.GoString(m.Send("body")))
				}
			}},
		}
		for _, m := range methods {
			if err := def.AddMethod(m.sel, m.enc, m.fn); err != nil {
				return
			}
		}
		if _, err := def.Register(); err != nil {
			return
		}
		delegateObj = def.New()
	})
	return delegateObj
}

// macError reads an NSError as a Go error, preferring the localised
// description because that is the only part of a WebKit failure a user could
// act on ("A server with the specified hostname could not be found").
func macError(err objc.ID) error {
	if !err.Valid() {
		return errors.New("desktop: the platform view failed to load; AppKit gave no reason")
	}
	if s := objc.GoString(err.Send("localizedDescription")); s != "" {
		return errors.New(s)
	}
	return errors.New("desktop: the platform view failed to load")
}

// --- objc_msgSend with a CGRect argument -------------------------------------

// cgRect mirrors CGRect: four CGFloats, which are doubles on every
// architecture macOS still runs.
type cgRect struct{ x, y, w, h float64 }

// internal/objc's Arg carries a type and a pointer and has no way to describe
// a struct passed by value, which -setFrame: and -initWithFrame:configuration:
// both need. Rather than widen that package — it is shared by every macOS
// capability and a struct-argument path is only wanted here — this file makes
// the one extra call shape itself, through the same pure-Go FFI and the same
// objc_msgSend symbol. Selector registration, string and array bridging and
// every other message still go through internal/objc.
var (
	msgSendOnce sync.Once
	msgSendSym  unsafe.Pointer
	msgSendErr  error
)

func msgSend() (unsafe.Pointer, error) {
	msgSendOnce.Do(func() {
		lib, err := ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
		if err != nil {
			msgSendErr = err
			return
		}
		msgSendSym, msgSendErr = ffi.GetSymbol(lib, "objc_msgSend")
	})
	return msgSendSym, msgSendErr
}

// cgRectType describes CGRect to the FFI: four doubles, 32 bytes. The ABI
// rules for passing it differ by architecture (registers on arm64, memory on
// amd64 for a struct this size) and goffi applies them from this description,
// which is why the layout has to be spelled out rather than inferred.
var cgRectType = &types.TypeDescriptor{
	Size:      32,
	Alignment: 8,
	Kind:      types.StructType,
	Members: []*types.TypeDescriptor{
		types.DoubleTypeDescriptor,
		types.DoubleTypeDescriptor,
		types.DoubleTypeDescriptor,
		types.DoubleTypeDescriptor,
	},
}

// sendRect sends sel with a CGRect by value, followed by up to two object
// arguments. The return is read as an object pointer even for a void method,
// which is harmless — the register is simply ignored — and keeps this to one
// call shape.
//
// Every argument lives in one heap-allocated box. That is not tidiness: an
// address handed to the FFI must stay put for the duration of the call, and a
// stack local can move when the call's own allocations grow the goroutine
// stack, which shows up as a zero handle next to a success return.
func sendRect(obj objc.ID, sel string, r cgRect, objs ...objc.ID) objc.ID {
	sym, err := msgSend()
	if err != nil || !obj.Valid() || len(objs) > 2 {
		return 0
	}
	s := objc.Sel(sel)
	if s == 0 {
		return 0
	}

	argTypes := make([]*types.TypeDescriptor, 0, 5)
	argTypes = append(argTypes,
		types.PointerTypeDescriptor, // self
		types.PointerTypeDescriptor, // _cmd
		cgRectType,
	)
	for range objs {
		argTypes = append(argTypes, types.PointerTypeDescriptor)
	}
	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, types.PointerTypeDescriptor, argTypes); err != nil {
		return 0
	}

	box := &struct {
		self uintptr
		cmd  uintptr
		rect cgRect
		tail [2]uintptr
		out  uintptr
	}{self: uintptr(obj), cmd: uintptr(s), rect: r}
	ptrs := make([]unsafe.Pointer, 0, 5)
	ptrs = append(ptrs,
		unsafe.Pointer(&box.self),
		unsafe.Pointer(&box.cmd),
		unsafe.Pointer(&box.rect),
	)
	for i, o := range objs {
		box.tail[i] = uintptr(o)
		ptrs = append(ptrs, unsafe.Pointer(&box.tail[i]))
	}
	if _, err := ffi.CallFunction(cif, sym, unsafe.Pointer(&box.out), ptrs); err != nil {
		return 0
	}
	out := objc.ID(box.out)
	runtime.KeepAlive(box)
	return out
}

// Both faces of the capability, checked at compile time.
// shell/coverage_test.go finds the accessor by scanning source, which cannot
// tell whether it actually satisfies the interface; these can.
var (
	_ shell.PlatformViewWindow = (*window)(nil)
	_ shell.WebViewWindow      = (*window)(nil)
	_ shell.PlatformViewHandle = (*macPlatformView)(nil)
)
