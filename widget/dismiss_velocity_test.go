package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Release speed comes from the frame clock, so the same gesture reads the same
// way however the platform batched its pointer events.
//
// It used to come from wall-clock stamps between those events. The interval
// between two events is not the interval the motion is drawn over — it varies
// with input batching — so an identical finger movement could read as a flick
// or as a nudge depending on timing the gesture had no control over. Headless
// it was worse than that: events dispatched microseconds apart looked like a
// flick whatever the distance, so the tests here had to sleep to be honest.
func dismissProbe(t *testing.T, steps int, dxPer float32) bool {
	t.Helper()
	dismissed := false
	root := widget.Dismissible{
		OnDismissed: func() { dismissed = true },
		Child: widget.Sized{W: 300, H: 80,
			Child: widget.Decorated{Color: paint.RGB(0.4, 0.4, 0.4)}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	x := float32(150)
	h.Press(geom.Pt{X: x, Y: 40})
	for range steps {
		x += dxPer
		h.Move(geom.Pt{X: x, Y: 40})
		h.Step(1.0 / 60) // one frame per move: the clock the speed is read on
		h.Render()
	}
	h.Release(geom.Pt{X: x, Y: 40})
	for range 120 {
		h.Step(1.0 / 60)
		h.Render()
	}
	return dismissed
}

func TestAFlickDismissesAndANudgeDoesNot(t *testing.T) {
	// 20pt per frame is 1200pt/s — past the 900 flick threshold — over a
	// distance well short of the 0.4 width the threshold path needs.
	if !dismissProbe(t, 4, 20) {
		t.Error("a fast flick did not dismiss")
	}
	// The same total distance, spread over five times as many frames: 240pt/s,
	// nowhere near a flick, and still short of the threshold.
	if dismissProbe(t, 20, 4) {
		t.Error("a slow drag over the same distance dismissed; it read as a flick")
	}
}
