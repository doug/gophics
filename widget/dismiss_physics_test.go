package widget

import (
	"testing"
	"time"

	"github.com/doug/gophics/anim"
)

// Letting go of a card must not change how fast it is moving.
//
// The release used a fixed duration whatever the gesture, so a hard flick
// visibly slowed the instant the finger lifted — the animation restarted at
// distance/duration regardless of the speed already on the card — and a small
// nudge crawled back over the same long interval. Timing the remaining travel
// to the release speed keeps the motion continuous, which is the property that
// reads as physical.
func TestReleaseDurationFollowsSpeed(t *testing.T) {
	const dist = 300

	fast := releaseDuration(0, dist, 3000) // hard flick
	slow := releaseDuration(0, dist, 700)  // gentle push

	if fast >= slow {
		t.Errorf("a flick at 3000px/s finishes in %v and a push at 700px/s in %v; "+
			"the faster gesture must finish sooner or releasing visibly slows the card",
			fast, slow)
	}

	// Same speed, different distances: the shorter travel finishes sooner.
	near := releaseDuration(0, 20, 1200)
	far := releaseDuration(0, 300, 1200)
	if near >= far {
		t.Errorf("20px takes %v and 300px takes %v at the same speed; distance "+
			"must matter or a small spring-back crawls", near, far)
	}
}

// The bounds keep both extremes usable: a hard flick still takes long enough to
// be seen, and a card released almost at rest does not drift.
func TestReleaseDurationStaysWithinBounds(t *testing.T) {
	cases := []struct{ from, to, vel float32 }{
		{0, 400, 50000}, // absurd flick
		{0, 2, 10},      // barely moved, barely moving
		{0, 0, 0},       // no travel at all
		{300, 0, -4000}, // springing back leftward
	}
	for _, c := range cases {
		d := releaseDuration(c.from, c.to, c.vel)
		if d < minReleaseMS*time.Millisecond {
			t.Errorf("from=%v to=%v v=%v gave %v, under the %dms floor — it would "+
				"finish inside a frame and read as a jump", c.from, c.to, c.vel, d, minReleaseMS)
		}
		if d > maxReleaseMS*time.Millisecond {
			t.Errorf("from=%v to=%v v=%v gave %v, over the %dms ceiling — the card "+
				"would drift", c.from, c.to, c.vel, d, maxReleaseMS)
		}
	}
}

// Direction must not change the timing: a card thrown left and one thrown right
// at the same speed take the same time.
func TestReleaseDurationIgnoresDirection(t *testing.T) {
	right := releaseDuration(0, 300, 1500)
	left := releaseDuration(0, -300, -1500)
	if right != left {
		t.Errorf("rightward %v but leftward %v; the same gesture mirrored should "+
			"take the same time", right, left)
	}
}

// The release must actually be wired to the animation, not merely computable.
//
// The tests above exercise releaseDuration on its own, so deleting the line in
// animateTo that applies it leaves them all passing — the card would go back to
// a fixed duration with nothing to notice. This drives animateTo and reads the
// duration off the controller it set.
func TestAnimateToAppliesTheReleaseDuration(t *testing.T) {
	newState := func(dx, vel float32) *dismissState {
		s := &dismissState{dx: dx, velocity: vel}
		s.anim = &anim.Controller{Duration: 999 * time.Millisecond}
		return s
	}

	run := func(s *dismissState, to float32) time.Duration {
		// animateTo ends by asking the context for a repaint, and this state was
		// never mounted, so that last call panics. The duration is set before
		// it, which is what is being checked.
		defer func() { _ = recover() }()
		s.animateTo(to, false)
		return 0
	}

	fast := newState(0, 4000)
	_ = run(fast, 300)
	slow := newState(0, 700)
	_ = run(slow, 300)

	if fast.anim.Duration == 999*time.Millisecond {
		t.Fatal("animateTo left the controller's duration untouched — the release " +
			"timing is computed and then thrown away")
	}
	if fast.anim.Duration >= slow.anim.Duration {
		t.Errorf("flick got %v and gentle push got %v; animateTo is not passing the "+
			"velocity through", fast.anim.Duration, slow.anim.Duration)
	}
}
