package widget_test

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A card that does not travel far enough springs back past its place.
//
// Easing back to rest reads as dead weight: the card decelerates into position
// and stops, like something being lowered. A spring arrives by going a little
// too far and returning, which is what the gesture is supposed to feel like on
// a Mac. So the test is not "does it return" — an ease-out returns too — but
// "does it cross".
func TestAShortSwipeSpringsBackPastItsPlace(t *testing.T) {
	var dismissed bool
	root := widget.Dismissible{
		OnDismissed: func() { dismissed = true },
		Child: widget.Sized{W: 300, H: 80,
			Child: widget.Semantics{Label: "CARD", Child: widget.Decorated{Color: paint.RGB(0.4, 0.4, 0.4)}}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	left := func() float32 {
		for _, n := range h.Semantics() {
			if n.Label == "CARD" {
				return n.Rect.Min.X
			}
		}
		t.Fatal("no card")
		return 0
	}
	rest := left()

	// A short, slow drag: well under the 0.4 threshold and nowhere near a
	// flick, so it must come back rather than dismiss.
	// Paced like a finger: Dismissible measures release speed off the wall
	// clock, so moves dispatched microseconds apart look like a flick however
	// short the distance.
	h.Press(geom.Pt{X: 150, Y: 40})
	for _, x := range []float32{160, 175, 190} {
		time.Sleep(30 * time.Millisecond)
		h.Move(geom.Pt{X: x, Y: 40})
		h.Render()
	}
	time.Sleep(30 * time.Millisecond)
	if left() <= rest {
		t.Fatal("the card did not follow the finger; nothing is being released")
	}
	h.Release(geom.Pt{X: 190, Y: 40})

	// Follow the spring home and watch for it crossing to the other side.
	crossed := false
	for i := 0; i < 120; i++ {
		h.Step(1.0 / 60)
		h.Render()
		if left() < rest-0.5 {
			crossed = true
		}
	}
	if dismissed {
		t.Fatal("a short slow swipe dismissed the card")
	}
	if !crossed {
		t.Error("the card eased back to rest without ever passing it; that is a stop, not a spring")
	}
	if got := left(); got < rest-0.5 || got > rest+0.5 {
		t.Errorf("the card settled at %v, want back at %v", got, rest)
	}
}

// A card on its way out only ever goes outward.
//
// The spring that makes the return feel right would, applied here, carry the
// card past the edge and bring it back a few pixels before it finished — and
// with a Background revealed behind it, that shows as a flicker at the edge
// just as the card leaves. Dismissal eases out instead.
func TestADismissedCardNeverComesBack(t *testing.T) {
	var dismissed bool
	root := widget.Dismissible{
		OnDismissed: func() { dismissed = true },
		Child: widget.Sized{W: 300, H: 80,
			Child: widget.Semantics{Label: "CARD", Child: widget.Decorated{Color: paint.RGB(0.4, 0.4, 0.4)}}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	left := func() (float32, bool) {
		for _, n := range h.Semantics() {
			if n.Label == "CARD" {
				return n.Rect.Min.X, true
			}
		}
		return 0, false
	}

	// Past the threshold, so this dismisses however fast the finger was.
	h.Press(geom.Pt{X: 20, Y: 40})
	for _, x := range []float32{80, 150, 220, 280} {
		time.Sleep(20 * time.Millisecond)
		h.Move(geom.Pt{X: x, Y: 40})
		h.Render()
	}
	h.Release(geom.Pt{X: 280, Y: 40})

	prev, ok := left()
	if !ok {
		t.Fatal("the card vanished before it was released")
	}
	for i := 0; i < 120; i++ {
		h.Step(1.0 / 60)
		h.Render()
		cur, ok := left()
		if !ok {
			break // gone from the tree
		}
		if cur < prev-0.5 {
			t.Fatalf("the card moved back from %v to %v on its way out", prev, cur)
		}
		prev = cur
	}
	if !dismissed {
		t.Error("a swipe past the threshold did not dismiss")
	}
}
