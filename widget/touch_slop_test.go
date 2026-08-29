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

// A finger is not a mouse, and the slop has to allow for that.
//
// One threshold governed taps, long-presses and drag commitment: 4 logical px
// of movement cancelled all three. That is a reasonable figure for a mouse,
// which sits still, and much too tight for a finger, which rolls a few points
// through any tap and drifts further through a half-second hold. The symptoms
// are not obviously connected to each other: rows that do not respond to being
// tapped, and text selection that never starts because its long press keeps
// being cancelled.
func tapProbe(t *testing.T, drift float32, touch bool, holdFrames int) (tapped, longPressed bool) {
	t.Helper()
	root := widget.Interactive{
		Gestures: widget.Gestures{
			OnTap:       func() { tapped = true },
			OnLongPress: func() { longPressed = true },
		},
		Child: widget.Sized{W: 200, H: 200, Child: widget.Decorated{Color: paint.RGB(0.5, 0.5, 0.5)}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 300}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	from := geom.Pt{X: 100, Y: 100}
	to := geom.Pt{X: 100 + drift, Y: 100}
	// The drift happens *during* the hold, which is what a finger does: it
	// does not wait for the long-press timer before moving.
	press, move, release := h.Press, h.Move, h.Release
	if touch {
		press, move, release = h.TouchPress, h.TouchMove, h.TouchRelease
	}
	press(from)
	for s := 0; s < holdFrames/3; s++ {
		h.Step(1.0 / 60)
	}
	move(to)
	for s := 0; s < holdFrames-holdFrames/3; s++ {
		h.Step(1.0 / 60)
	}
	release(to)
	_ = time.Second
	return tapped, longPressed
}

func TestATouchTapToleratesAFingerRolling(t *testing.T) {
	// A quick tap — held well under the long-press threshold, since a long
	// press deliberately consumes the tap and the two must not be conflated.
	// 6pt of drift is within what a finger does on a deliberate tap, and past
	// the 4pt a mouse is held to.
	if tapped, _ := tapProbe(t, 6, true, 6); !tapped {
		t.Error("a touch tap with 6pt of drift did not register")
	}
	// A real drag still has to cancel the tap, or nothing could ever scroll.
	if tapped, _ := tapProbe(t, 40, true, 6); tapped {
		t.Error("a 40pt touch drag registered as a tap")
	}
}

func TestALongPressSurvivesAFingerDrifting(t *testing.T) {
	// The long press fires while held, before the drift; what matters is that
	// the drift does not retroactively cancel it.
	if _, long := tapProbe(t, 6, true, 45); !long {
		t.Error("a touch long-press with 6pt of drift did not fire")
	}
}

// A mouse keeps its tighter threshold: it does not drift, and a small movement
// there is a deliberate drag.
func TestAMouseKeepsTheTighterSlop(t *testing.T) {
	if tapped, _ := tapProbe(t, 6, false, 6); tapped {
		t.Error("a 6px mouse drag registered as a tap; the mouse slop should stay tight")
	}
}
