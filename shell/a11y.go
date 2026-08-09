package shell

import "github.com/doug/gophics/geom"

// Accessibility capability: bridges gophics's own-rendered UI to the platform
// assistive technologies (VoiceOver, TalkBack, Narrator, screen readers).
// Because gophics draws every pixel, nothing is accessible by default — the
// app's semantics must be published here. A Window exposes it by implementing
// AccessibilityWindow; widgets reach it via ctx.Accessibility(), nil where
// unsupported.
//
// STATUS: Announce (transient live-region speech) is implemented on web and is
// the first thing to wire everywhere. SetTree — publishing the full explorable
// accessibility tree so an AT user can navigate the UI — is the large per-
// platform bridge (a DOM/ARIA mirror on web, an NSAccessibility/Accessibility
// NodeInfo tree on native) and is the main remaining a11y work; see
// docs/design-capabilities.md.

// AccessibilityWindow is implemented by a Window that can talk to the platform
// assistive technologies.
type AccessibilityWindow interface {
	Accessibility() Accessibility
}

// Accessibility publishes UI semantics to platform assistive technologies.
type Accessibility interface {
	// Announce speaks message through the AT immediately. assertive interrupts
	// current speech (for errors/alerts); otherwise it's queued politely.
	Announce(message string, assertive bool)
	// SetTree publishes the current accessibility tree for the AT to explore.
	SetTree(root A11yNode)
}

// A11yNode is one node of the accessibility tree: a role, an accessible label,
// its bounds in surface pixels, and children. Roles follow the ARIA vocabulary
// ("button", "checkbox", "heading", "textbox", …) so every platform can map
// from one source.
type A11yNode struct {
	Role     string
	Label    string
	Bounds   geom.Rect
	Children []A11yNode
}
