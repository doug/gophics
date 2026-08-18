package shell

// Window-control capability. A Window exposes it by implementing
// WindowControlWindow; callers reach it through the widget layer
// (ctx.WindowControl()), which returns nil when the running platform can't
// provide one. It exists mainly for desktop developer tooling — set the window
// title, toggle fullscreen, and read the current size — with a best-effort web
// mapping onto the equivalent DOM APIs. It is not offered on mobile, where the
// OS owns window chrome.
//
// This is deliberately a *small, honest* interface: every method is backed by a
// real platform primitive on at least desktop, and each reports whether the
// platform did the thing rather than assuming.
//
// SetSize reports false where the windowing system owns geometry — Wayland,
// where a toplevel is told its size, and the browser, which refuses resizeTo
// for anything but a popup it opened.
//
// Tray and native menus have their own capabilities now (shell/tray.go,
// shell/menu.go) rather than living here: both outlive any one window, which is
// the whole point of a tray icon and is true of a macOS menu bar too.

// WindowControlWindow is implemented by a Window that can control its own
// window chrome. The app runner type-asserts the Window to it and, when present,
// publishes WindowControl() to the widget tree — the same shape as
// FilePickerWindow/MediaWindow.
type WindowControlWindow interface {
	WindowControl() WindowControl
}

// WindowControl adjusts and inspects the host window. All methods are
// synchronous and must be called from the UI goroutine.
type WindowControl interface {
	// SetTitle sets the window (desktop) or document (web) title.
	SetTitle(string)
	// SetFullscreen enters (true) or exits (false) fullscreen. On web the first
	// entry must happen inside a user gesture — the browser rejects
	// requestFullscreen otherwise.
	SetFullscreen(bool)
	// Fullscreen reports whether the window is currently fullscreen.
	Fullscreen() bool
	// Size returns the current window size in logical pixels.
	Size() (w, h float32)
	// SetSize requests a new window content size in logical pixels, reporting
	// whether the platform applied it.
	//
	// False is a real answer, not an error: Wayland gives the compositor
	// ownership of window geometry, and a browser refuses resizeTo for anything
	// but a popup it opened. Callers should treat a resize affordance as
	// unavailable when this returns false rather than leaving a control that
	// silently does nothing. Even true is a request — a window manager may
	// clamp it.
	SetSize(w, h float32) bool
}
