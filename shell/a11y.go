package shell

// Accessibility capability: bridges gophics's own-rendered UI to the platform
// assistive technologies (VoiceOver, TalkBack, Narrator, screen readers).
// Because gophics draws every pixel, nothing is accessible by default — the
// app's semantics must be published here. A Window exposes it by implementing
// AccessibilityWindow; widgets reach it via ctx.Accessibility(), nil where
// unsupported.
//
// STATUS, by platform:
//
//   - web — complete. Announce speaks through an aria-live region; SetTree
//     maintains an explorable ARIA DOM mirror over the canvas.
//   - macOS — SetTree publishes NSAccessibilityElements on the content view,
//     so VoiceOver can explore and activate the UI. Announce is not wired yet
//     (AppKit routes live-region speech through a C function rather than an
//     Objective-C method); see the gogpu darwin bridge for the detail.
//   - Linux, Windows, iOS, Android — not implemented. Accessibility() returns
//     nil there rather than a sink that silently discards, so a caller can
//     tell the difference.
//
// Every bridge consumes the same A11yNode slice; see
// docs/design-capabilities.md.

// AccessibilityWindow is implemented by a Window that can talk to the platform
// assistive technologies.
type AccessibilityWindow interface {
	Accessibility() Accessibility
}

// A11yProvider is the pull side of the same bridge, implemented by the app's
// Handler. Push (Accessibility.SetTree) suits a platform whose AT wants to be
// handed a tree — the web, where the mirror is a DOM. Pull suits the native
// APIs, which call *into* the app on their own schedule: AppKit asks a view
// for its accessibility children when VoiceOver moves, and Android's
// AccessibilityNodeProvider is queried node by node.
//
// A platform bridge type-asserts its Handler to this and answers those queries
// from the result. Both sides deliver the same A11yNode values, so a platform
// picks whichever shape its AT wants without a second tree format.
//
// All calls must happen on the UI goroutine.
type A11yProvider interface {
	// A11yTree returns the current tree, with bounds scaled to physical
	// pixels by the given device scale.
	A11yTree(scale float32) []A11yNode
	// A11yActivate performs the node's default action. Unknown or
	// non-actionable IDs are ignored.
	A11yActivate(id int)
	// A11yHitTest returns the ID of the deepest meaningful node at a physical
	// pixel point, or -1 — the query behind explore-by-touch.
	A11yHitTest(x, y int, scale float32) int
}

// Accessibility publishes UI semantics to platform assistive technologies.
type Accessibility interface {
	// Announce speaks message through the AT immediately. assertive interrupts
	// current speech (for errors/alerts); otherwise it's queued politely.
	Announce(message string, assertive bool)
	// SetTree publishes the current accessibility tree for the AT to explore.
	// nodes is a flattened tree in creation order; nodes[i].Children holds
	// child IDs, and exactly one node has ParentID -1.
	//
	// activate is called with a node ID when the AT activates that node (a
	// VoiceOver double-tap, a TalkBack tap, an Enter on a focused element).
	// It is delivered on the UI goroutine. The implementation must retain the
	// callback until the next SetTree replaces it.
	//
	// Publishing an empty slice tears the tree down.
	SetTree(nodes []A11yNode, activate func(id int))
}

// A11yNode is one node of a flat, ID-addressed accessibility tree — the shape
// every platform bridge consumes (an ARIA DOM mirror on web, Android's
// AccessibilityNodeProvider, iOS UIAccessibilityElement, AccessKit). Roles
// follow the ARIA vocabulary ("button", "checkbox", "heading", "textbox", …)
// so one source feeds every platform. Bounds are physical pixels.
type A11yNode struct {
	ID       int
	ParentID int // -1 for the root
	Role     string
	Label    string
	Value    string
	Hint     string
	// X, Y, W, H are physical-pixel bounds.
	X, Y, W, H int
	Tappable   bool
	Focused    bool
	Disabled   bool
	Selected   bool
	Checkable  bool
	Checked    bool
	Children   []int
}
