package ui

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// Resizing across the contentMaxW threshold must not throw the reader back to
// the section list. capWidth wraps the Navigator, and it used to return a bare
// child below the threshold and a Padding above it: the subtree changed shape
// as the window crossed 760px, reconciliation gave the child a new element, and
// the Navigator remounted with an empty stack. Dragging a window edge past that
// width closed whatever section was open.
//
// The check watches homeHook rather than the Nav handle taken at boot. That
// handle keeps reporting depth 2 after a remount because it refers to the
// discarded navigator state — it cannot see the bug. homePage.Build runs on the
// replacement, so a fresh handle at depth 1 is the remount itself.
func TestResizeAcrossWidthCapKeepsThePage(t *testing.T) {
	h, nav, _ := startHome(t)

	nav.Push(sections()[0].page())
	settle(h)
	if d := nav.Depth(); d != 2 {
		t.Fatalf("push did not take: nav depth = %d, want 2", d)
	}

	remounted := false
	homeHook = func(n widget.Nav) {
		if n.Depth() == 1 {
			remounted = true
		}
	}
	defer func() { homeHook = nil }()

	// 420 -> 900 crosses contentMaxW (760), then back under, then over again.
	for _, w := range []float32{900, 420, 1200} {
		h.Resize(geom.Size{W: w, H: 760})
		settle(h)
		if remounted {
			t.Fatalf("resizing to %.0f remounted the Navigator — the open section "+
				"was replaced by the catalog list", w)
		}
	}
}
