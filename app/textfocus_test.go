package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

type twoFields struct{}

func (twoFields) Build(widget.Ctx) widget.Widget {
	field := func() widget.Widget {
		return widget.Sized{W: 200, H: 40, Child: widget.TextField{Value: ""}}
	}
	return widget.Column(
		field(),
		widget.Sized{H: 20},
		field(),
		widget.Sized{H: 200, Child: widget.Decorated{Color: paint.RGB(0.9, 0.9, 0.9)}},
	)
}

func newFields(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(twoFields{}, Config{Size: geom.Size{W: 300, H: 400}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

// Tapping outside a text field must release it, because on a phone that is the
// only way to put the soft keyboard away.
//
// Focus used to be sticky: a press that hit nothing focusable left the field
// focused, so the keyboard stayed up covering half the screen with no gesture
// that could dismiss it.
func TestTappingOutsideAFieldReleasesIt(t *testing.T) {
	h := newFields(t)
	h.Tap(geom.Pt{X: 100, Y: 20}) // into the first field
	h.Render()
	if h.core.Owner.KeyboardTarget == nil {
		t.Fatal("tapping the field did not focus it")
	}

	h.Tap(geom.Pt{X: 100, Y: 300}) // empty space below
	h.Render()
	if h.core.Owner.KeyboardTarget != nil {
		t.Error("the field kept focus after a tap outside it — on a phone the " +
			"keyboard would stay up with no way to dismiss it")
	}
}

// Moving between two fields must land on the second, not leave nothing focused.
func TestTappingASecondFieldMovesFocus(t *testing.T) {
	h := newFields(t)
	h.Tap(geom.Pt{X: 100, Y: 20})
	h.Render()
	first := h.core.Owner.KeyboardTarget
	if first == nil {
		t.Fatal("no focus after tapping the first field")
	}

	h.Tap(geom.Pt{X: 100, Y: 80}) // the second field
	h.Render()
	second := h.core.Owner.KeyboardTarget
	if second == nil {
		t.Fatal("tapping the second field left nothing focused")
	}
	if second == first {
		t.Error("focus stayed on the first field")
	}
}

// Released focus must stay released across rebuilds.
//
// The implicit autofocus rule — "a focusable widget mounted while nothing has
// focus takes it" — was evaluated on every rebuild rather than on mount, so any
// focusable in the tree grabbed focus back the frame after anything released
// it. Releasing lasted exactly one frame, which on a phone meant the keyboard
// could not be dismissed: the first field re-took focus and raised it again
// immediately.
func TestReleasedFocusSurvivesRebuilds(t *testing.T) {
	h := newFields(t)
	h.Tap(geom.Pt{X: 100, Y: 20})
	h.Render()
	if h.core.Owner.KeyboardTarget == nil {
		t.Fatal("no focus after tapping the field")
	}

	h.Tap(geom.Pt{X: 100, Y: 200}) // outside every field
	h.Render()

	// Several frames, each of which rebuilds and re-runs the autofocus rule.
	for range 5 {
		h.core.Owner.RebuildAll()
		h.Render()
		if h.core.Owner.KeyboardTarget != nil {
			t.Fatal("a focusable re-took focus after it was released; the " +
				"keyboard would come straight back up")
		}
	}
}

// The first field on a screen still takes focus when it mounts, which is what
// makes a form usable without a tap. Releasing must not cost that.
func TestFirstFieldStillAutofocusesOnMount(t *testing.T) {
	h := newFields(t)
	if h.core.Owner.KeyboardTarget == nil {
		t.Error("no field took focus on mount; a form would need a tap before " +
			"the keyboard appeared")
	}
}
