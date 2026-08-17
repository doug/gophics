package platform

// Accessibility. A window that draws all its own pixels is, to the operating
// system's assistive technologies, a single opaque image: there are no native
// controls to inspect, so nothing is reachable unless the application says
// what it drew. This is the channel for saying it.
//
// The node shape is deliberately flat and ID-addressed rather than a pointer
// tree, because that is what every platform API on the other side wants — an
// AppKit accessibility element list, an Android AccessibilityNodeProvider
// keyed by virtual view id, an AccessKit node map.

// A11yNode is one accessibility node. Bounds are in physical pixels with the
// origin at the top-left of the window's client area — the coordinate system
// the drawing code already works in. Each platform converts to whatever its
// own accessibility API expects.
type A11yNode struct {
	ID       int
	ParentID int // -1 for the root
	// Role is an ARIA role name ("button", "checkbox", "slider", …). One
	// vocabulary is used across platforms and mapped at the edge.
	Role  string
	Label string
	Value string
	Hint  string
	// X, Y, W, H are physical-pixel bounds.
	X, Y, W, H int
	Tappable   bool
	Focused    bool
	Disabled   bool
	Selected   bool
	Checkable  bool
	Checked    bool
}

// A11yWindow is implemented by a PlatformWindow that can publish an
// accessibility tree to the OS. It is an optional interface — a platform
// without a bridge simply does not implement it, and the caller degrades to
// publishing nothing rather than to a compile error.
type A11yWindow interface {
	// SetA11yTree replaces the published tree. activate is invoked, on the
	// platform's own thread, when assistive technology performs a node's
	// default action; the caller is responsible for marshalling that onto
	// whatever goroutine its own state lives on.
	//
	// Publishing an empty slice tears the tree down and hands the window back
	// to the platform's default (opaque) representation.
	SetA11yTree(nodes []A11yNode, activate func(id int))

	// AnnounceA11y speaks a transient message through the assistive
	// technology without changing the tree — a live region. assertive
	// interrupts current speech; otherwise the message is queued politely.
	AnnounceA11y(message string, assertive bool)
}

// A11yAvailable refines A11yWindow for platforms whose bridge may be absent at
// run time rather than at compile time.
//
// Implementing A11yWindow is a static claim, and on macOS it is the whole
// story: if the binary is running there, AppKit is there too. Linux is not like
// that. AT-SPI lives on a second D-Bus that exists only when the desktop has
// accessibility enabled, so the same binary has a bridge on one machine and
// none on the next. Without this, such a platform would have to either claim a
// bridge it cannot use — leaving callers with a capability that silently
// discards everything, which is worse than an absent one because an app cannot
// tell the difference — or never claim one at all.
type A11yAvailable interface {
	// A11yAvailable reports whether the bridge can actually publish. A window
	// implementing this is treated as having no accessibility support while it
	// returns false.
	A11yAvailable() bool
}
