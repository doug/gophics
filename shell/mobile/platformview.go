package mobile

import (
	"errors"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// Native subviews on iOS and Android: shell.PlatformViews over a host
// interface, in the same shape as every other host role here.
//
// The shape is forced, not chosen. gomobile can carry bool, the numeric types,
// string, []byte and named types from this package; a method it cannot
// represent is dropped from the generated host API without a word
// (bindable_test.go). So Go cannot be handed a UIView or a MapView and cannot
// pass one back: the only thing that crosses is a kind named as a string and an
// integer id, which is exactly why shell.PlatformViews is a kind registry
// rather than a capability per kind. See its doc comment for that argument,
// and for the two things a platform view can never do.
//
// Why the native leaf lives in the host project rather than here: a WKWebView,
// an MKMapView and an AVPlayerLayer are UIKit objects assembled in Swift,
// added to the host's own view hierarchy, and gophics has no business naming
// them — the same division as PreviewHost (Camera2/AVFoundation) and FileHost
// (the document pickers). What gophics owns is the rect and the lifetime.
//
// # The additive property, and why it is the point
//
// A host that does not register a PlatformViewHost is unchanged in every
// respect. ctx.PlatformViews() is nil, widget.PlatformView paints its
// placeholder, and an existing Android activity that keeps setContentView(v)
// compiles and runs exactly as before. gophics never sees the container the
// host puts the views in, never requires one to exist, and never reaches up
// into the hierarchy: the host decides where a subview goes and does its own
// addView / addSubview. That is what makes this safe to add to a shipped app —
// the feature is opt-in at the host, not a new requirement on every host.
//
// # Threading
//
// Everything here is UI thread, with no exception of the kind PreviewHost's
// frame delivery has. CreateView, SetViewBounds, SendToView and DestroyView
// all touch the native view hierarchy, which both platforms require on the
// main thread; DeliverViewReady, FailView and DeliverViewMessage run app
// callbacks that mutate the widget tree. There is no high-rate path to make an
// exception for — SetViewBounds fires when the layout moves, not per frame.
//
// # Coordinates
//
// Bounds cross in physical pixels, the same units as SetSurface and SetInsets,
// because that is the side of the logical/physical line the Bridge is on
// (AGENTS.md). Android view geometry is already in pixels, so a host uses them
// directly; iOS frames are in points, so an iOS host divides by the screen
// scale it passed to SetSurface. Mixing the two is invisible at 1x and wrong
// at 2x or 3x, which is every modern phone.

// PlatformViewHost is implemented by the native host (Swift on iOS, Kotlin on
// Android) and registered with Bridge.SetPlatformViewHost.
//
// Every method is called on the UI thread. A host that is asked for a kind it
// did not register should answer FailView rather than ignoring the call: a
// widget waiting on a view that will never arrive shows a loading state
// forever, which is the one outcome worse than a failure.
type PlatformViewHost interface {
	// CreateView builds a native view of kind and adds it to the host's own
	// hierarchy, above the gophics view. params is the kind's configuration —
	// a URL for "webview", "lat,lon,zoom" for "map" — and is whatever the Go
	// side passed; the host defines it per kind.
	//
	// Answer with DeliverViewReady(id) once the view has content, or
	// FailView(id, msg). Exactly one, and not before returning from this call.
	CreateView(id int, kind, params string)
	// SetViewBounds moves and resizes the view, in physical pixels. An empty
	// rect — w or h zero — means hide it without destroying it, so that a
	// view scrolled out of a list keeps its state for when it scrolls back.
	// Called whenever the layout moves it, which is not every frame.
	SetViewBounds(id, x, y, w, h int)
	// SendToView passes a message to the view: a URL to navigate to, "pause".
	// The format is the host's business per kind. A host with nothing to
	// receive may ignore it.
	SendToView(id int, msg string)
	// DestroyView removes the view from the hierarchy and releases it. It may
	// arrive for an id that already failed, and must tolerate that.
	DestroyView(id int)
}

// SetPlatformViewHost registers the native platform-view backend, enabling
// ctx.PlatformViews(). kinds is a comma-separated list of the view kinds this
// host can build ("webview,map") — comma-separated because gomobile cannot
// bind a []string parameter, the same reason FileHost.PickFiles takes its
// accept list that way.
//
// Until a host is registered the capability is nil, which is how an app hides
// the affordance instead of opening a view that never appears. Registering
// with an empty kinds list is a host saying "I can host views but build none
// of them": the capability is published and Kinds() is empty, which an app can
// tell apart from nil — see shell.PlatformViews.Kinds.
func (b *Bridge) SetPlatformViewHost(h PlatformViewHost, kinds string) {
	b.pvHost = h
	b.pvKinds = nil
	for _, k := range strings.Split(kinds, ",") {
		if k = strings.TrimSpace(k); k != "" {
			b.pvKinds = append(b.pvKinds, k)
		}
	}
	b.capabilitiesChanged()
}

// PlatformViews makes the Bridge a shell.PlatformViewWindow. Nil until a host
// registers; see SetPlatformViewHost.
func (b *Bridge) PlatformViews() shell.PlatformViews {
	if b.pvHost == nil {
		return nil
	}
	return mobilePlatformViews{b}
}

type mobilePlatformViews struct{ b *Bridge }

// Kinds returns a copy. The slice is handed to app code, which may keep it,
// and a later SetPlatformViewHost would otherwise rewrite what the app is
// holding.
func (p mobilePlatformViews) Kinds() []string {
	return append([]string(nil), p.b.pvKinds...)
}

func (p mobilePlatformViews) Open(kind, params string, bounds geom.Rect, h shell.PlatformViewHandlers) shell.PlatformViewHandle {
	b := p.b
	known := false
	for _, k := range b.pvKinds {
		if k == kind {
			known = true
			break
		}
	}
	if !known {
		// A kind this host did not register. Nil rather than a handle that
		// will report a failure: the widget treats both the same way, and a
		// nil keeps the "a listed kind opens" contract checkable.
		return nil
	}
	id := b.newReq()
	if b.pvCb == nil {
		b.pvCb = map[int]shell.PlatformViewHandlers{}
	}
	b.pvCb[id] = h
	b.pvHost.CreateView(id, kind, params)
	hdl := &mobilePlatformViewHandle{b: b, id: id}
	hdl.SetBounds(bounds)
	return hdl
}

type mobilePlatformViewHandle struct {
	b      *Bridge
	id     int
	closed bool
}

func (h *mobilePlatformViewHandle) SetBounds(r geom.Rect) {
	if h.closed {
		return
	}
	s := h.b.scale
	if s <= 0 {
		s = 1
	}
	if r.IsEmpty() {
		// One canonical "hidden" rather than whichever degenerate rect the
		// layout produced, so a host can test w==0 and be done.
		h.b.pvHost.SetViewBounds(h.id, 0, 0, 0, 0)
		return
	}
	px := func(v float32) int { return int(v*s + 0.5) }
	h.b.pvHost.SetViewBounds(h.id, px(r.Min.X), px(r.Min.Y), px(r.Dx()), px(r.Dy()))
}

func (h *mobilePlatformViewHandle) Send(msg string) {
	if h.closed {
		return
	}
	h.b.pvHost.SendToView(h.id, msg)
}

func (h *mobilePlatformViewHandle) Close() {
	if h.closed {
		return
	}
	h.closed = true
	delete(h.b.pvCb, h.id)
	h.b.pvHost.DestroyView(h.id)
}

// DeliverViewReady reports that the view for id has content to show. Call on
// the UI thread.
func (b *Bridge) DeliverViewReady(id int) {
	cb := b.pvCb[id].OnReady
	if cb != nil {
		cb()
	}
}

// FailView reports that the view for id could not be created or loaded. The
// handle is dead afterwards; the widget still calls DestroyView on unmount, so
// a host must tolerate that. Call on the UI thread.
func (b *Bridge) FailView(id int, msg string) {
	cb := b.pvCb[id].OnError
	if cb != nil {
		cb(errors.New(msg))
	}
}

// DeliverViewMessage carries a message from the view to the app — a navigation
// finishing, a marker tapped, a video ending. Call on the UI thread.
func (b *Bridge) DeliverViewMessage(id int, msg string) {
	cb := b.pvCb[id].OnMessage
	if cb != nil {
		cb(msg)
	}
}

// WebView makes the Bridge a shell.WebViewWindow, derived from the registry
// rather than implemented separately: a web view is a kind of platform view.
// Nil unless a host registered one that offers the "webview" kind, so
// ctx.WebView() stays an honest nil on the overwhelming majority of apps,
// which host nothing.
func (b *Bridge) WebView() shell.WebView { return shell.WebViewOf(b.PlatformViews()) }
