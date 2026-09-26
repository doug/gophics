package widget_test

import (
	"fmt"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// A LazyList learns its viewport's extent from layout, not only from a scroll
// event. Before this the window was 800px until something scrolled, so a
// viewport taller than that — a desktop window, a tablet — showed rows down
// to 1200px and blank space below until the user scrolled.

func labelledRows() widget.LazyList {
	return widget.LazyList{Count: 200, EstimatedExtent: 48, Build: func(i int) widget.Widget {
		return widget.Semantics{Label: fmt.Sprintf("row%d", i), Child: widget.Sized{H: 48}}
	}}
}

func hasSemLabel(h *app.Headless, label string) bool {
	var walk func(nodes []layout.SemNode) bool
	walk = func(nodes []layout.SemNode) bool {
		for _, n := range nodes {
			if n.Label == label || walk(n.Children) {
				return true
			}
		}
		return false
	}
	return walk(h.Semantics())
}

func TestLazyListFillsTallViewportOnFirstFrame(t *testing.T) {
	h := headless(t, labelledRows(), 320, 1600)
	h.Render()
	// Row 30 sits at y=1440: inside the 1600px viewport, past the 800px guess
	// and its overscan.
	if !hasSemLabel(h, "row30") {
		t.Fatal("row30 (y=1440) not mounted in a 1600px viewport on the first frame")
	}
	if hasSemLabel(h, "row60") {
		t.Fatal("row60 (y=2880) mounted: the window is not bounded by the viewport")
	}
}

func TestLazyListFollowsResize(t *testing.T) {
	h := headless(t, labelledRows(), 320, 240)
	h.Render()
	h.Move(geom.Pt{X: 160, Y: 120})
	h.Scroll(geom.Pt{Y: -10}) // a scroll fixes the extent at 240px
	h.Render()
	if hasSemLabel(h, "row30") {
		t.Fatal("row30 mounted in a 240px viewport")
	}
	h.Resize(geom.Size{W: 320, H: 1600})
	h.Render()
	if !hasSemLabel(h, "row30") {
		t.Fatal("row30 (y=1440) not mounted after resizing to 1600px")
	}
}
