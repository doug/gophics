package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

type flingList struct{ c *widget.ScrollController }

func (f flingList) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 200)
	for i := range rows {
		rows[i] = widget.Sized{W: 300, H: 30, Child: widget.Decorated{Color: paint.RGB(0.8, 0.4, 0.2)}}
	}
	return widget.Scroll{Controller: f.c, Child: widget.Column(rows...)}
}

func newFlingList(t *testing.T) (*Headless, *widget.ScrollController) {
	t.Helper()
	c := &widget.ScrollController{}
	h, err := NewHeadless(flingList{c: c}, Config{Size: geom.Size{W: 300, H: 400}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, c
}

// drag moves the finger by dy per frame for n frames, then lifts.
func drag(h *Headless, from geom.Pt, dy float32, n int) {
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: from})
	p := from
	for range n {
		p.Y += dy
		h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
		h.Step(1.0 / 60)
	}
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
}

// settle runs frames until nothing is animating.
func settleFrames(h *Headless, t *testing.T) int {
	t.Helper()
	for i := range 900 {
		if !h.Step(1.0 / 60) {
			return i
		}
	}
	t.Fatal("never settled")
	return 0
}

// A flick keeps going after the finger lifts, and how far depends only on the
// distance covered per frame.
//
// This could not be tested at all before. Velocity was measured between pointer
// events with time.Now() and samples closer together than a millisecond were
// discarded, so synthetic events — which arrive microseconds apart — produced a
// velocity of exactly zero and no fling. The one existing fling test worked
// around it with time.Sleep between moves, which made it a test of the wall
// clock as much as of the physics.
func TestFlickFlingsFromTheFrameClock(t *testing.T) {
	h, c := newFlingList(t)
	drag(h, geom.Pt{X: 150, Y: 350}, -30, 6) // 30px per frame upward = fast
	atRelease := c.Offset()
	settleFrames(h, t)

	if atRelease <= 0 {
		t.Fatalf("the drag itself did not scroll (offset %v)", atRelease)
	}
	if c.Offset() <= atRelease {
		t.Errorf("no fling: offset %v at release, %v after settling", atRelease, c.Offset())
	}
}

// A finger that stops and holds before lifting must not fling.
//
// The old estimate decayed only when a pointer event arrived, so a finger held
// still produced no events and kept whatever velocity it had when it stopped —
// stop dead, lift, and the list flew off anyway. Sampling per frame means a
// still finger contributes zero-velocity samples and the estimate decays,
// which is what holding still means.
func TestHoldingStillBeforeLiftingDoesNotFling(t *testing.T) {
	h, c := newFlingList(t)

	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 150, Y: 350}})
	p := geom.Pt{X: 150, Y: 350}
	for range 6 { // move fast...
		p.Y -= 30
		h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
		h.Step(1.0 / 60)
	}
	for range 12 { // ...then hold still, without lifting
		h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
		h.Step(1.0 / 60)
	}
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})

	atRelease := c.Offset()
	settleFrames(h, t)

	if drift := c.Offset() - atRelease; drift > 1 {
		t.Errorf("a finger held still before lifting still flung %v px", drift)
	}
}

// Faster flicks travel further — the estimate tracks speed rather than just
// reporting "some".
func TestFasterFlickTravelsFurther(t *testing.T) {
	slowH, slow := newFlingList(t)
	drag(slowH, geom.Pt{X: 150, Y: 350}, -10, 6)
	slowRelease := slow.Offset()
	settleFrames(slowH, t)
	slowFling := slow.Offset() - slowRelease

	fastH, fast := newFlingList(t)
	drag(fastH, geom.Pt{X: 150, Y: 350}, -40, 6)
	fastRelease := fast.Offset()
	settleFrames(fastH, t)
	fastFling := fast.Offset() - fastRelease

	if fastFling <= slowFling {
		t.Errorf("fast flick coasted %v px, slow one %v — velocity is not tracking speed",
			fastFling, slowFling)
	}
}

// The same flick must fling the same distance whatever the refresh rate.
//
// The estimate is smoothed with a time constant rather than a per-sample
// weight, because a per-sample weight is a different filter at every refresh
// rate — 0.3 a frame converges in half the wall time at 120Hz as at 60Hz, so
// one flick would carry differently on two phones.
func TestFlingIsIndependentOfRefreshRate(t *testing.T) {
	// Same gesture in wall-clock terms: 180px over 100ms, sampled two ways.
	run := func(hz float64, frames int, dyPerFrame float32) float32 {
		h, c := newFlingList(t)
		dt := 1 / hz
		h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 150, Y: 350}})
		p := geom.Pt{X: 150, Y: 350}
		for range frames {
			p.Y -= dyPerFrame
			h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
			h.Step(dt)
		}
		h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
		atRelease := c.Offset()
		for range 900 {
			if !h.Step(dt) {
				break
			}
		}
		return c.Offset() - atRelease
	}

	at60 := run(60, 6, 30)    // 6 frames x 30px = 180px in 100ms
	at120 := run(120, 12, 15) // 12 frames x 15px = 180px in 100ms

	if at60 == 0 || at120 == 0 {
		t.Fatalf("no fling at one of the rates: 60Hz %v, 120Hz %v", at60, at120)
	}
	ratio := at120 / at60
	t.Logf("60Hz coasted %.1f, 120Hz coasted %.1f, ratio %.3f", at60, at120, ratio)
	// Tight on purpose. A per-sample weight instead of a time constant
	// measures 1.11 here, and a tolerance loose enough to admit that would
	// not be testing anything.
	if ratio < 0.95 || ratio > 1.05 {
		t.Errorf("the same flick coasted %v at 60Hz and %v at 120Hz (ratio %.2f) — "+
			"the velocity filter is refresh-rate dependent", at60, at120, ratio)
	}
}
