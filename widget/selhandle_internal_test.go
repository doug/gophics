package widget

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// The handle geometry and the drag that follows it, tested from inside the
// package because the grips are painted rather than laid out: they have no box
// and no semantics node, so nothing outside can find them without guessing at
// pixels — and a test that guesses is a test that passes for the wrong reason.
//
// The end-to-end behaviour it supports (long-press, then Copy) is covered from
// outside in editmenu_test.go.

func handleFixture(t *testing.T, value string, a, b int) (*textFieldState, *paint.Painter) {
	t.Helper()
	pr := paint.NewPainter()
	if err := pr.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	s := &textFieldState{}
	s.ed.SetText(value)
	s.ed.MoveTo(a, false)
	s.ed.MoveTo(b, true)
	s.handles = true
	s.dragHandle = -1
	return s, pr
}

// A selection with no handles showing offers nothing to grab, which is what
// keeps a mouse user from finding invisible hit zones under their text.
func TestHandlesOnlyWhenRaisedByTouch(t *testing.T) {
	s, pr := handleFixture(t, "alpha bravo charlie", 6, 11)

	if _, _, ok := s.handleCentres(pr); !ok {
		t.Fatal("a touch selection must offer handles")
	}
	s.handles = false
	if _, _, ok := s.handleCentres(pr); ok {
		t.Error("handles offered for a selection no finger made")
	}
	s.handles = true
	s.ed.MoveTo(6, false) // collapse
	if _, _, ok := s.handleCentres(pr); ok {
		t.Error("handles offered for an empty selection")
	}
}

// The two grips sit at the two ends, below the text — below, because a finger
// on the grip must not cover the character it is positioning.
func TestHandlesSitAtTheSelectionEnds(t *testing.T) {
	s, pr := handleFixture(t, "alpha bravo charlie", 6, 11)
	lo, hi, ok := s.handleCentres(pr)
	if !ok {
		t.Fatal("no handles")
	}
	if lo.X >= hi.X {
		t.Errorf("handles at x=%v and x=%v are not in order", lo.X, hi.X)
	}
	if lo.Y <= 0 || hi.Y <= 0 {
		t.Errorf("handles at y=%v/%v are not below the text", lo.Y, hi.Y)
	}
	// They track the text: a selection further along sits further right.
	s2, _ := handleFixture(t, "alpha bravo charlie", 12, 19)
	lo2, _, _ := s2.handleCentres(pr)
	if lo2.X <= lo.X {
		t.Errorf("a later selection starts at x=%v, not right of x=%v", lo2.X, lo.X)
	}
}

// A press within the grab radius takes the nearer handle; one outside it takes
// none, and the caller places the caret instead.
func TestHandleGrabRadius(t *testing.T) {
	s, pr := handleFixture(t, "alpha bravo charlie", 6, 11)
	lo, hi, _ := s.handleCentres(pr)

	if got := s.handleAtPt(pr, lo); got != 0 {
		t.Errorf("press on the start grip picked %d, want 0", got)
	}
	if got := s.handleAtPt(pr, hi); got != 1 {
		t.Errorf("press on the end grip picked %d, want 1", got)
	}
	far := geom.Pt{X: lo.X, Y: lo.Y + selHandleGrab*3}
	if got := s.handleAtPt(pr, far); got != -1 {
		t.Errorf("press well away from any grip picked %d, want -1", got)
	}
}

// Dragging a grip past the other one inverts the selection rather than
// collapsing it — which is what someone dragging it expects, and the reason the
// drag tracks an explicit anchor instead of reusing the caret's extend flag.
func TestHandleDragCrossingSwapsEnds(t *testing.T) {
	s, _ := handleFixture(t, "alpha bravo charlie", 6, 11)
	s.dragHandle = 1 // dragging the end

	// Pull the end back past the start.
	s.moveHandleTo(2)

	a, b := s.ed.Selection()
	if a == b {
		t.Fatal("the selection collapsed when a handle crossed the other")
	}
	if a != 2 || b != 6 {
		t.Errorf("selection = (%d,%d), want (2,6) — the dragged end became the start", a, b)
	}
	if s.dragHandle != 0 {
		t.Errorf("still dragging handle %d after crossing; it became the start", s.dragHandle)
	}
}
