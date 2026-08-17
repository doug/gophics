package gogpu

import "github.com/doug/gophics/internal/gfx/gogpu/internal/platform"

// A11yNode is one node of an accessibility tree published to the operating
// system's assistive technologies. Bounds are physical pixels with the origin
// at the top-left of the window's client area.
//
// It is an alias for the platform-level type so no conversion happens on the
// way down; callers outside this module use it by this name.
type A11yNode = platform.A11yNode

// SetAccessibilityTree publishes the window's accessibility tree.
//
// A gogpu window paints every pixel itself, so to a screen reader it is one
// unlabeled image until the application describes what it drew. Calling this
// with a non-empty slice replaces the published description; calling it with
// an empty slice removes it.
//
// activate is invoked when assistive technology performs a node's default
// action — VoiceOver's VO-Space on a button. It arrives on the platform's UI
// thread, which is not necessarily the goroutine the caller's state lives on;
// marshal it if that matters.
//
// It reports whether the running platform has an accessibility bridge. false
// means the tree was not published and nothing was changed, so a caller can
// decide between staying quiet and telling the user their screen reader will
// not see this window. On Linux that answer depends on the machine, not just
// the build: AT-SPI is only there when the desktop has accessibility enabled.
func (a *App) SetAccessibilityTree(nodes []A11yNode, activate func(id int)) bool {
	w, ok := a.platWindow.(platform.A11yWindow)
	if !ok {
		return false
	}
	if !a11yAvailable(a.platWindow) {
		return false
	}
	w.SetA11yTree(nodes, activate)
	return true
}

// a11yAvailable asks a platform that can lose its bridge at run time — Linux,
// whose AT-SPI bus exists only when the desktop has accessibility enabled —
// whether it has one right now. Platforms that do not implement the probe are
// taken at their word.
func a11yAvailable(w any) bool {
	p, ok := w.(platform.A11yAvailable)
	return !ok || p.A11yAvailable()
}

// AnnounceAccessibility speaks a transient message through the assistive
// technology without changing the tree — the live-region idiom, for things
// like "5 results" after a search. assertive interrupts current speech.
//
// It reports whether the platform has a bridge, on the same terms as
// SetAccessibilityTree.
func (a *App) AnnounceAccessibility(message string, assertive bool) bool {
	w, ok := a.platWindow.(platform.A11yWindow)
	if !ok {
		return false
	}
	if !a11yAvailable(a.platWindow) {
		return false
	}
	w.AnnounceA11y(message, assertive)
	return true
}
