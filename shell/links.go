package shell

// Links capability: the URL the app was launched/navigated to (deep links) and
// notification of inbound URLs delivered while running. A Window opts in by
// implementing LinksWindow; widgets reach it via ctx.Links(), nil where
// unsupported. Callbacks fire on the UI goroutine.
//
// This is the inbound counterpart to Window.OpenURL (which sends a URL out to
// the system browser): Links reports URLs that arrive *into* the app — the page
// address on web, a launch argument on desktop, a deep-link Intent / universal
// link on mobile.

// LinksWindow is implemented by a Window that can report inbound/launch URLs.
// The app runner type-asserts it and publishes Links() to the widget tree — the
// same shape as ConnectivityWindow/HapticWindow.
type LinksWindow interface {
	Links() Links
}

// Links reports the launch URL and inbound URL deliveries.
type Links interface {
	// Initial returns the URL the app was launched or navigated to, or "" if
	// there was none. On web this is the current page address; on desktop it is
	// the first launch argument that parses as a URL/path; on mobile it is the
	// deep link the app was opened with.
	Initial() string
	// OnLink registers f, called with each URL delivered to the app while it is
	// running (a web history navigation, a mobile deep link handed off by the
	// host). Some platforms cannot observe in-session deliveries (see the
	// per-platform docs) and never invoke f — use Initial() for the launch URL
	// in that case.
	OnLink(func(url string))
}
