package widget_test

import (
	"testing"
	"time"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// A JumpTo must report the offset it jumps to and nothing else.
//
// It reported the previous AnimateTo target first: stopping the glide went
// through Jump, whose OnChange applies the glide's end value and reports it.
// So a feed that had glided to its bottom and was then jumped back to the top
// heard "at the bottom" once more and loaded another page.
func TestJumpToDoesNotReportTheLastGlideTarget(t *testing.T) {
	ctrl := &widget.ScrollController{}
	ends := 0
	var offsets []float32
	h := headless(t, widget.Scroll{
		Child:        widget.Sized{W: 300, H: 2000},
		Controller:   ctrl,
		OnEndReached: func() { ends++ },
		OnOffset:     func(off, _ float32) { offsets = append(offsets, off) },
	}, 320, 240)
	h.Render()
	bottom := ctrl.MaxOffset()
	ctrl.AnimateTo(bottom, 100*time.Millisecond)
	for range 5 {
		h.Step(0.05)
		h.Render()
	}
	if ctrl.Offset() != bottom {
		t.Fatalf("offset after the glide = %v, want %v", ctrl.Offset(), bottom)
	}
	if ends != 1 {
		t.Fatalf("OnEndReached fired %d times reaching the bottom, want 1", ends)
	}

	ctrl.JumpTo(0)
	h.Render()
	offsets = offsets[:0]
	ctrl.JumpTo(0) // already there: nothing for OnOffset or OnEndReached to say
	for _, o := range offsets {
		if o == bottom {
			t.Errorf("JumpTo(0) reported the stale glide target %v", o)
		}
	}
	if ends != 1 {
		t.Errorf("OnEndReached fired %d times; a jump to the top re-fired it", ends)
	}
}

// A Controller handed to a Scroll after it mounted is still a controller.
//
// It was bound only in Init, so a controller created on a later build — or
// swapped for another — was never attached, and its JumpTo went nowhere.
func TestScrollControllerSuppliedAfterMountBinds(t *testing.T) {
	var ctrl *widget.ScrollController
	root, m := newMutable(func(widget.Ctx) widget.Widget {
		return widget.Scroll{Child: widget.Sized{W: 300, H: 2000}, Controller: ctrl}
	})
	h := headless(t, root, 320, 240)
	h.Render()

	ctrl = &widget.ScrollController{}
	m.Rebuild()
	h.Render()
	ctrl.JumpTo(500)
	h.Render()
	if ctrl.Offset() != 500 {
		t.Fatalf("offset after JumpTo(500) = %v; the late controller was ignored", ctrl.Offset())
	}
}

// pullToRefresh drags the top of the list down past the trigger and lets go.
func pullToRefresh(t *testing.T, h *app.Headless) {
	t.Helper()
	h.TouchPress(geom.Pt{X: 160, Y: 20})
	h.TouchMove(geom.Pt{X: 160, Y: 40})
	h.Render()
	h.TouchMove(geom.Pt{X: 160, Y: 400})
	h.Render()
	h.TouchRelease(geom.Pt{X: 160, Y: 400})
	h.Render()
}

// The refresh spinner keeps turning for as long as the refresh runs.
//
// Its phase advanced on the ticker, but nothing asked for a frame and the
// indicator only copied the phase during a build. Once the band had settled
// there was no build, every frame came out identical and was skipped, and the
// spinner sat frozen until the app cleared Refreshing.
func TestRefreshSpinnerRepaintsWhileRefreshing(t *testing.T) {
	refreshing := false
	var m *mutableModel
	root, m0 := newMutable(func(widget.Ctx) widget.Widget {
		return widget.Scroll{
			Child:      widget.Sized{W: 300, H: 2000},
			Refreshing: refreshing,
			OnRefresh:  func() { refreshing = true; m.Rebuild() },
		}
	})
	m = m0
	h := headless(t, root, 320, 240)
	h.Render()
	pullToRefresh(t, h)
	if !refreshing {
		t.Fatal("OnRefresh did not fire")
	}
	// Let the snap-to-rest and the scrollbar fade finish.
	for range 30 {
		h.Step(0.1)
		h.Render()
	}
	if !h.Step(0.3) {
		t.Fatal("nothing is ticking while the refresh runs; the spinner has no clock")
	}
	h.Render()
	if h.Skipped() {
		t.Fatal("the frame after the spinner advanced was skipped as unchanged: the indicator never rotates")
	}
}

// Grabbing the list while the refresh indicator is retracting stops the
// retraction where it is; the finger owns the band from then on.
//
// A press stopped the fling and the overscroll spring but not the snap that
// retracts the indicator, so the snap kept writing the band's height under
// the drag's own writes and the content twitched between the two.
func TestGrabDuringRetractStopsTheSnap(t *testing.T) {
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
	pullToRefresh(t, h)
	for range 30 {
		h.Step(0.1)
		h.Render()
	}
	held := semRect(t, h, "content").Min.Y
	if held <= 0 {
		t.Fatalf("content top = %v after the pull; the band is not held open", held)
	}

	// The app finishes: the band starts retracting.
	refreshing = false
	m.Rebuild()
	h.Render()
	h.Step(0.05)
	h.Render()
	mid := semRect(t, h, "content").Min.Y
	if mid <= 0 || mid >= held {
		t.Fatalf("content top = %v mid-retract, want between 0 and %v", mid, held)
	}

	// A finger lands mid-retract and holds still.
	h.TouchPress(geom.Pt{X: 160, Y: 200})
	for range 10 {
		h.Step(0.05)
		h.Render()
	}
	if got := semRect(t, h, "content").Min.Y; got != mid {
		t.Fatalf("content top moved from %v to %v under a still finger: the snap kept running", mid, got)
	}
	h.TouchRelease(geom.Pt{X: 160, Y: 200})
}
