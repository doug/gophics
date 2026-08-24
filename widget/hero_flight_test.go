package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A hero flight is placed by the same slide fractions that place the pages.
//
// The flight interpolates between two rects recovered from an earlier paint.
// Recovering them means removing whatever slide each page carried when it was
// captured — a hero stops painting once it is flying, so its raw rect goes
// stale and cannot be used directly. Removing the slide and stopping there put
// the element where its page was *going* to be: for the whole transition the
// hero sat offset from its own page by the page's remaining travel, meeting it
// only on the final frame. So the slide has to go back on, at this frame's
// value, which is what pageFracs exists to keep in one place.

func TestPageFracsLeaveTheArrivingPageAtRest(t *testing.T) {
	// The arriving page must be exactly at rest when the transition ends,
	// because that is what makes a hero land on it rather than beside it.
	if over, _ := pageFracs(1, false); over != 0 {
		t.Errorf("push ended with the incoming page at frac %v, want 0", over)
	}
	if _, under := pageFracs(1, true); under != 0 {
		t.Errorf("pop ended with the revealed page at frac %v, want 0", under)
	}
}

func TestPopResumesWhereThePushEnded(t *testing.T) {
	// A pop starts from the arrangement a push finished in; if it did not, the
	// pages (and any hero riding on them) would jump on the first frame back.
	pushOver, pushUnder := pageFracs(1, false)
	popOver, popUnder := pageFracs(0, true)
	if pushOver != popOver || pushUnder != popUnder {
		t.Errorf("push ended at (over=%v, under=%v) but pop starts at (over=%v, under=%v)",
			pushOver, pushUnder, popOver, popUnder)
	}

	// And the mirror: a push starts where a pop finishes.
	pushOver0, pushUnder0 := pageFracs(0, false)
	popOver1, popUnder1 := pageFracs(1, true)
	if pushOver0 != popOver1 || pushUnder0 != popUnder1 {
		t.Errorf("push starts at (over=%v, under=%v) but pop ends at (over=%v, under=%v)",
			pushOver0, pushUnder0, popOver1, popUnder1)
	}
}

func TestRemovingAPageSlideAndPuttingItBackIsIdentity(t *testing.T) {
	// The flight normalises a painted rect to at-rest and then re-applies the
	// current slide. When the two fracs agree the rect must come back exactly,
	// or a hero would drift by the difference every frame it flies.
	rc := geom.RectXYWH(40, 120, 60, 60)
	const width = 420
	// Exact equality is too strong: frac*width is computed twice and float32
	// rounding leaves ~1e-5 of a pixel behind. A tenth of a pixel is the
	// threshold that matters — drift the eye could ever see.
	near := func(a, b float32) bool { return a-b < 0.1 && b-a < 0.1 }
	for _, frac := range []float32{0, 1, -0.3, 0.42} {
		got := restRect(rc, frac, width).Translate(geom.Pt{X: frac * width})
		if !near(got.Min.X, rc.Min.X) || !near(got.Min.Y, rc.Min.Y) ||
			!near(got.Max.X, rc.Max.X) || !near(got.Max.Y, rc.Max.Y) {
			t.Errorf("frac %v: round-tripped %v to %v, want it unchanged", frac, rc, got)
		}
	}
}
