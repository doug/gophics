package main

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// During a push both pages move: the incoming one crosses the surface while the
// outgoing one eases aside.
//
// Only the top page used to move, so a page appeared to pop on top of a fixed
// background rather than a stack sliding — reported as pages popping over from
// the right instead of feeling like a transition. The outgoing page travels a
// fraction of the width, so the two separate as they go.
func TestPushMovesTheOutgoingPageToo(t *testing.T) {
	h, nav, _ := startHome(t)
	openFeed(t, h, nav)

	// Where a feed row sits before anything moves.
	before, ok := firstRowX(h.Semantics())
	if !ok {
		t.Fatal("no feed rows on screen to measure")
	}

	// Push the detail page and stop part-way into the slide.
	h.Tap(geom.Pt{X: 210, Y: 220})
	for i := 0; i < 6; i++ { // ~100ms of 220ms
		h.Step(1.0 / 60)
	}

	during, ok := firstRowX(h.Semantics())
	if !ok {
		t.Fatal("the outgoing page vanished mid-transition")
	}

	if during == before {
		t.Errorf("the outgoing page is still at x=%.0f part-way through the push; "+
			"the incoming page is sliding over a static background", before)
	}
	if during > before {
		t.Errorf("the outgoing page moved right (%.0f -> %.0f); it should ease "+
			"left, away from the page arriving from the right", before, during)
	}
}

// firstRowX returns the x of the first labelled row in the tree.
func firstRowX(nodes []layout.SemNode) (float32, bool) {
	for _, n := range layout.FlattenSemantics(nodes) {
		if n.Label != "" && n.Rect.Dx() > 40 {
			return n.Rect.Min.X, true
		}
	}
	return 0, false
}
