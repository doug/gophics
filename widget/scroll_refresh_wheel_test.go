package widget_test

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// A wheel scroll while a refresh is held leaves the band alone: the spinner
// is showing for a refresh that is still running, and a drag already leaves
// it in place. OnScroll used to zero the band regardless, so the spinner
// vanished while Refreshing was still true.
func TestWheelLeavesHeldRefreshBand(t *testing.T) {
	refreshing := false
	var m *mutableModel
	root, m0 := newMutable(func(widget.Ctx) widget.Widget {
		return widget.Scroll{
			Child:      widget.Semantics{Label: "content", Child: widget.Sized{W: 300, H: 2000}},
			Refreshing: refreshing,
			OnRefresh:  func() { refreshing = true; m.Rebuild() },
		}
	})
	m = m0
	h := headless(t, root, 320, 240)
	h.Render()
	h.TouchPress(geom.Pt{X: 160, Y: 20})
	h.TouchMove(geom.Pt{X: 160, Y: 40})
	h.Render()
	h.TouchMove(geom.Pt{X: 160, Y: 400})
	h.Render()
	h.TouchRelease(geom.Pt{X: 160, Y: 400})
	h.Render()
	if !refreshing {
		t.Fatal("pulling past the trigger did not fire OnRefresh")
	}
	for range 30 {
		h.Step(0.1)
		h.Render()
	}
	held := semRect(t, h, "content").Min.Y
	if held <= 0 {
		t.Fatalf("band not held while refreshing (content top %v)", held)
	}
	h.Move(geom.Pt{X: 160, Y: 120})
	h.Scroll(geom.Pt{Y: -10})
	h.Render()
	for range 10 {
		h.Step(0.1)
		h.Render()
	}
	if got := semRect(t, h, "content").Min.Y; got <= 0 {
		t.Errorf("a wheel scroll collapsed the held band (content top %v -> %v) while the refresh is still running", held, got)
	}
}
