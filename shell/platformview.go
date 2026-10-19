package shell

import "github.com/doug/gophics/geom"

// PlatformViews capability: embed a real native view — a web view, a map, a
// video surface, an ad unit — inside a gophics layout. A Window opts in by
// implementing PlatformViewWindow; widgets reach it via ctx.PlatformViews(),
// nil where unsupported. Use widget.PlatformView rather than this interface
// directly: the widget owns the handle for its element's lifetime and pushes
// bounds from the layout pass, which is the part that is tedious and easy to
// leak.
//
// This is the capability that deliberately breaks the "gophics draws every
// pixel" model. The content is not in the scene, not in the damage model, and
// not drawn by the painter: it is a sibling view the OS composites, positioned
// in gophics coordinates. Everything else here is an escape hatch of the
// ordinary kind, where the platform answers a question. This one hands a
// rectangle of the screen to somebody else.
//
// # Two things are impossible, permanently
//
// These are not backlog. They are consequences of how an OS composites a
// foreign view, and an app that needs either of them needs a different design,
// not a later version of this capability.
//
// A platform view cannot be interleaved between two widget layers. The OS
// composites it above (or below) the framework's whole surface, so there is no
// "put the map under this sheet and over that list" — it is one view, above
// everything gophics draws, clipped to a rect. Buying real z-order means
// hybrid composition: on Android, turning the direct-present SurfaceView into
// a TextureView and reordering real Android views, at a cost Flutter documents
// and that would give up the entire mobile GPU story (shell/mobile/gpu.go).
// Draw the chrome that must sit on top with native views inside the hosted
// view, or move it out from over the view entirely.
//
// A platform view can never appear in an offscreen snapshot. Bridge.Snapshot
// (shell/mobile/mobile.go) rasterises a frame on the CPU and the OS compositor
// is not involved, so the view is simply not there. Concretely: every
// golden-image test of a screen containing a platform view will show a hole
// where the view is, with whatever the widget tree painted underneath it
// showing through — the Placeholder, if widget.PlatformView was given one.
// This is worth being blunt about because first-class headless testing is one
// of this project's real advantages over Flutter, and this is the one feature
// that erodes it. A screen with a platform view on it is a screen whose
// appearance is partly untested. Keep the view's rect small, give it a
// Placeholder so the hole is deliberate rather than whatever was behind it,
// and test the surrounding layout on a screen that does not open one.
//
// The texture route — the host streaming the native view's pixels into the
// widget tree, as shell.CameraPreview already does for the camera — is the one
// route that avoids both limits, at the cost of a full-surface copy per frame.
// It is not closed off. It is not this capability.
//
// # Why a kind registry and not a capability per kind
//
// The alternative design was typed capabilities — shell.MapView,
// shell.VideoView, shell.WebView — one per kind of view. That is how the rest
// of shell/ is shaped and it is self-documenting, so it deserves an answer.
//
// The decision is one opaque registry, for three reasons that compound.
//
// The bind boundary does not carry types anyway. gomobile can represent bool,
// the numeric types, string and []byte, plus named types from the bound
// package; it silently drops a method it cannot represent
// (shell/mobile/bindable_test.go). So Go cannot hand a host a MapView or
// receive one — the only shape that crosses is "name a kind as a string, get
// an integer id back", which is how FileHost, MediaHost, PreviewHost,
// MonitorHost, ShareHost, NotifyHost, SecureHost, LocationHost and DeviceHost
// all already work. A typed capability per kind would funnel through exactly
// this string-and-id plumbing underneath, so the typing would be a Go-side
// veneer over an untyped boundary rather than a contract the platform honours.
// It buys N interfaces, N host registrations and N posted wrappers, and zero
// additional checking at the point where the mistake actually happens.
//
// Typing is still available, and it is no longer a core change. widget's
// NewCapability (widget/extension_gen.go) lets a package in another module
// declare a capability and its <X>Window opt-in without patching this tree. So
// a maps library can publish a typed maps.View with structured parameters,
// implemented over this registry, and an app reads it with the same nil check
// it uses for a built-in. The typed/opaque question used to be a fork in the
// road because every kind had to land here; since the extension point shipped
// it is a question of which layer, and the lower layer is the one that has to
// be untyped.
//
// Kinds is a better answer than nil-per-kind. With typed capabilities, "this
// build has no map" and "this platform has no platform views at all" are the
// same nil. Here they are distinguishable: PlatformViews() == nil means the
// platform cannot composite a foreign view (desktop today, the terminal
// forever), while a non-nil registry whose Kinds does not list "map" means a
// host is present and did not implement that one. An app showing a degraded
// static map image wants to tell those apart.
//
// The cost is real and worth stating: kind and params are strings, so the
// contract for a given kind lives in documentation rather than in the type
// system, and a typo is a runtime OnError rather than a compile error. The
// mitigation is that params is opaque to gophics — a host that wants structure
// can take JSON — and that a kind with a typed wrapper around it (see above)
// gets its checking back at the layer where someone can maintain it.
//
// # What a host must do
//
// A platform view's native leaf is host code: WKWebView and MKMapView on iOS,
// WebView and MapView on Android, an <iframe> on web. gophics never names a
// native class. On mobile the host implements mobile.PlatformViewHost and
// registers it with Bridge.SetPlatformViewHost, naming the kinds it can build;
// until it does, ctx.PlatformViews() is nil and an app hides the affordance.
// That is the normal case, not a failure: most apps embed nothing, and an
// existing host that never registers one compiles and runs exactly as before.

// PlatformViewWindow is implemented by a Window that can host native subviews.
// The app runner type-asserts the Window to it and publishes PlatformViews()
// to the widget tree — the same shape as SocketWindow/FilePickerWindow.
type PlatformViewWindow interface {
	PlatformViews() PlatformViews
}

// PlatformViews is the registry of native view kinds this build can embed.
type PlatformViews interface {
	// Kinds reports the view kinds the host can build, e.g. "webview", "map",
	// "video". An app asks before offering an affordance: a non-nil registry
	// that does not list "map" means a host is present and cannot build that
	// one, which is a different answer from a nil capability (this platform
	// cannot embed anything) and leads to a different fallback.
	//
	// The result is advisory in one direction only: a kind that is listed may
	// still fail to open (no network, a missing API key), reported through
	// PlatformViewHandlers.OnError. A kind that is not listed will not open.
	Kinds() []string

	// Open creates a view of kind at bounds and returns a handle immediately.
	// The handle is live — SetBounds and Close work on it at once — but the
	// *content* is not: creation is asynchronous everywhere (a page navigates,
	// a map loads tiles, a video buffers) and the outcome arrives on h.OnReady
	// or h.OnError, exactly one of which fires, both on the UI goroutine.
	//
	// params is opaque to gophics and interpreted by the host for that kind: a
	// URL for "webview", a coordinate and zoom for "map", a media id for
	// "video". A host that wants structured parameters should document JSON.
	//
	// bounds is in the same logical coordinates the widget tree uses; the
	// shell maps them to wherever its surface sits on screen. An empty rect
	// is legal and means "hide": a view scrolled out of its viewport is hidden
	// rather than closed, so its state (scroll position, a half-filled form,
	// a playing video) survives being scrolled back into view.
	//
	// Open never returns nil for a kind Kinds listed; it may return nil for a
	// kind it did not.
	Open(kind, params string, bounds geom.Rect, h PlatformViewHandlers) PlatformViewHandle
}

// PlatformViewHandlers are the lifecycle callbacks for one opened view. Any
// field may be nil. All fire on the UI goroutine (see PostedPlatformViews).
type PlatformViewHandlers struct {
	// OnReady fires once, when the view has content to show.
	OnReady func()
	// OnError fires instead of OnReady when the view could not be created or
	// its content could not be loaded. The handle is dead afterwards; Close is
	// still safe, and is still the widget's job.
	OnError func(err error)
	// OnMessage carries a message from the view to the app — a navigation
	// finishing, a marker tapped, a video ending. Its format is the host's
	// business for that kind, as params is.
	OnMessage func(msg string)
}

// PlatformViewHandle controls one live native view. It carries no callbacks of
// its own (those were given to Open), so it needs no posted wrapper. Its
// methods are safe to call from the UI goroutine, and safe to call after the
// view has failed or been closed, where they do nothing — a widget unmounting
// should not have to know which.
type PlatformViewHandle interface {
	// SetBounds moves and resizes the view. An empty rect hides it without
	// destroying it; see Open.
	SetBounds(geom.Rect)
	// Send passes a message to the view — a URL to navigate to, a coordinate
	// to pan to, "pause". Its format is the host's business, as params is. A
	// host that offers nothing to send may ignore it.
	Send(msg string)
	// Close destroys the view. Idempotent. widget.PlatformView calls it on
	// unmount, which is the reason to prefer the widget: a handle opened by
	// hand outlives the element that wanted it, and the view stays on screen
	// over whatever replaced it.
	Close()
}

// WebViewOf adapts a PlatformViews registry to the older, narrower WebView
// capability, so a shell implements the registry once and gets both. It
// returns nil when pv is nil or does not offer the "webview" kind, which is
// what keeps ctx.WebView() an honest nil rather than a handle to nothing.
//
// Both names are supported because a web view is a kind of platform view, not
// a capability of its own, and treating "embed a browser" as the general
// problem was the mistake this replaces. New code should use
// widget.PlatformView with Kind PlatformViewWebView; WebView stays for callers
// that already exist and for the narrow case of genuinely only wanting a URL.
func WebViewOf(pv PlatformViews) WebView {
	if pv == nil {
		return nil
	}
	for _, k := range pv.Kinds() {
		if k == PlatformViewWebView {
			return webViewOverPlatformViews{pv}
		}
	}
	return nil
}

// The kinds gophics itself names. A host may offer kinds not listed here —
// that is the point of a registry — but these three have a documented params
// format so that an app and a host can agree without talking.
const (
	// PlatformViewWebView embeds a browser view; params is the URL to load,
	// and Send navigates to another URL.
	PlatformViewWebView = "webview"
	// PlatformViewMap embeds the platform map; params is "lat,lon,zoom".
	PlatformViewMap = "map"
	// PlatformViewVideo embeds the platform video player; params is the media
	// URL, and Send takes "play", "pause" or "seek:<milliseconds>".
	PlatformViewVideo = "video"
)

type webViewOverPlatformViews struct{ pv PlatformViews }

func (w webViewOverPlatformViews) Open(url string, bounds geom.Rect) WebViewHandle {
	h := w.pv.Open(PlatformViewWebView, url, bounds, PlatformViewHandlers{})
	if h == nil {
		return nil
	}
	return h
}
