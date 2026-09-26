package widget_test

import (
	"testing"
	"time"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// glideMidway starts an AnimateTo on a tall scroll and runs it part way,
// returning the app and the controller with the glide still in flight.
func glideMidway(t *testing.T) (*app.Headless, *widget.ScrollController) {
	t.Helper()
	ctrl := &widget.ScrollController{}
	h := headless(t, widget.Scroll{Child: widget.Sized{W: 300, H: 4000}, Controller: ctrl}, 320, 240)
	h.Render()
	ctrl.AnimateTo(2000, 2*time.Second)
	h.Step(0.2)
	h.Render()
	if mid := ctrl.Offset(); mid <= 0 || mid >= 2000 {
		t.Fatalf("offset %v after 0.2s of a 2s glide, want mid-glide", mid)
	}
	return h, ctrl
}

// A finger landing during an AnimateTo takes hold: the glide stops where the
// finger lands instead of scrolling on underneath it.
func TestGrabStopsGlide(t *testing.T) {
	h, ctrl := glideMidway(t)
	at := ctrl.Offset()
	h.TouchPress(geom.Pt{X: 160, Y: 120})
	for range 10 {
		h.Step(0.05)
		h.Render()
	}
	if got := ctrl.Offset(); got != at {
		t.Errorf("the glide kept scrolling under a still finger: %v -> %v", at, got)
	}
	h.TouchRelease(geom.Pt{X: 160, Y: 120})
}

// A wheel scroll during an AnimateTo wins: the glide does not put the offset
// back on its curve on the next tick.
func TestWheelStopsGlide(t *testing.T) {
	h, ctrl := glideMidway(t)
	h.Move(geom.Pt{X: 160, Y: 120})
	h.Scroll(geom.Pt{Y: 500}) // wheel back toward the top
	h.Render()
	after := ctrl.Offset()
	h.Step(0.05)
	h.Render()
	if got := ctrl.Offset(); got != after {
		t.Errorf("the glide overrode the wheel scroll: %v -> %v", after, got)
	}
}

// A scrollbar drag stops a glide too — one that starts mid-drag, from the
// app, since a press has already stopped any that was running.
func TestBarDragStopsGlide(t *testing.T) {
	ctrl := &widget.ScrollController{}
	h := headless(t, widget.Scroll{Child: widget.Sized{W: 300, H: 4000}, Controller: ctrl}, 320, 240)
	h.Render()
	// A scroll shows the bar and its paint measures the thumb; the build
	// after that places the drag target over it, at the top of the right
	// edge: 16pt wide, 240*240/4000 ≈ 14pt tall.
	h.Move(geom.Pt{X: 160, Y: 120})
	h.Scroll(geom.Pt{Y: -1})
	h.Render()
	h.Scroll(geom.Pt{Y: -1})
	h.Render()
	h.TouchPress(geom.Pt{X: 314, Y: 6})
	h.TouchMove(geom.Pt{X: 314, Y: 30}) // past slop: the thumb owns the drag
	h.Render()
	if ctrl.Offset() < 100 {
		t.Fatalf("offset %v after dragging the thumb 24pt, want the content to have moved with it", ctrl.Offset())
	}
	ctrl.AnimateTo(2000, 2*time.Second)
	h.TouchMove(geom.Pt{X: 314, Y: 40})
	h.Render()
	at := ctrl.Offset()
	for range 10 {
		h.Step(0.05)
		h.Render()
	}
	if got := ctrl.Offset(); got != at {
		t.Errorf("the glide kept scrolling under the held thumb: %v -> %v", at, got)
	}
	h.TouchRelease(geom.Pt{X: 314, Y: 40})
}
