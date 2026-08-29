package widget

import (
	"testing"
)

func dismissFixture(t *testing.T, w Dismissible) (*Owner, *dismissState) {
	t.Helper()
	o := newOwner()
	o.SetRoot(w)
	o.FlushBuilds()
	st := digState[Dismissible](o.root)
	if st == nil {
		t.Fatal("dismissible state not found")
	}
	return o, st.(*dismissState)
}

// pumpDismiss runs frames until the spring/dismiss animation settles.
func pumpDismiss(t *testing.T, o *Owner, s *dismissState) {
	t.Helper()
	for range 240 {
		o.TickAll(0.016)
		o.FlushBuilds()
		if !s.anim.Running() {
			return
		}
	}
	t.Fatal("dismiss animation did not settle")
}

// A short swipe (under the threshold, no flick) springs back to rest and
// does not dismiss.
func TestDismissibleShortSwipeSpringsBack(t *testing.T) {
	dismissed := false
	o, s := dismissFixture(t, Dismissible{
		Child:       Sized{W: 200, H: 40},
		OnDismissed: func() { dismissed = true },
	})
	s.width = 200 // normally captured from layout
	s.SetState(func() { s.dx = 40 })
	s.release()
	pumpDismiss(t, o, s)

	if s.dx != 0 {
		t.Fatalf("dx = %v after spring-back, want 0", s.dx)
	}
	if dismissed || s.gone {
		t.Fatal("short swipe must not dismiss")
	}
}

// A swipe past the threshold animates fully off (dx = width) and fires
// OnDismissed exactly once.
func TestDismissibleSwipePastThresholdDismisses(t *testing.T) {
	fired := 0
	o, s := dismissFixture(t, Dismissible{
		Child:       Sized{W: 200, H: 40},
		OnDismissed: func() { fired++ },
	})
	s.width = 200
	s.SetState(func() { s.dx = 120 }) // past the default 0.4*200 = 80
	s.release()
	pumpDismiss(t, o, s)

	if fired != 1 {
		t.Fatalf("OnDismissed fired %d times, want 1", fired)
	}
	if !s.gone || s.dx != 200 {
		t.Fatalf("gone=%v dx=%v, want gone at dx=width", s.gone, s.dx)
	}

	// A release after the dismiss is inert.
	s.release()
	pumpDismiss(t, o, s)
	if fired != 1 {
		t.Fatalf("OnDismissed re-fired after dismissal (%d)", fired)
	}
}

// A fast flick dismisses even when the displacement is small.
func TestDismissibleFlickDismisses(t *testing.T) {
	dismissed := false
	o, s := dismissFixture(t, Dismissible{
		Child:       Sized{W: 200, H: 40},
		OnDismissed: func() { dismissed = true },
	})
	s.width = 200
	s.SetState(func() { s.dx = 10 })
	s.velocity = 1500 // px/s, past the 900 flick threshold
	s.release()
	pumpDismiss(t, o, s)

	if !dismissed {
		t.Fatal("fast flick did not dismiss")
	}
}

// Direction constraints clamp the drag: a DismissRight row cannot be pushed
// left, and vice versa.
func TestDismissibleDirectionClamp(t *testing.T) {
	_, right := dismissFixture(t, Dismissible{Child: Sized{W: 200, H: 40}, Direction: DismissRight})
	if got := right.allow(-50); got != 0 {
		t.Fatalf("DismissRight allow(-50) = %v, want 0", got)
	}
	if got := right.allow(50); got != 50 {
		t.Fatalf("DismissRight allow(50) = %v, want 50", got)
	}

	_, left := dismissFixture(t, Dismissible{Child: Sized{W: 200, H: 40}, Direction: DismissLeft})
	if got := left.allow(50); got != 0 {
		t.Fatalf("DismissLeft allow(50) = %v, want 0", got)
	}
	if got := left.allow(-50); got != -50 {
		t.Fatalf("DismissLeft allow(-50) = %v, want -50", got)
	}
}

// A custom Threshold is honored.
func TestDismissibleCustomThreshold(t *testing.T) {
	dismissed := false
	o, s := dismissFixture(t, Dismissible{
		Child:       Sized{W: 200, H: 40},
		Threshold:   0.8,
		OnDismissed: func() { dismissed = true },
	})
	s.width = 200
	// 120px is past the default 0.4 threshold but short of 0.8*200 = 160.
	s.SetState(func() { s.dx = 120 })
	s.release()
	pumpDismiss(t, o, s)
	if dismissed {
		t.Fatal("swipe under the custom threshold dismissed")
	}
	if s.dx != 0 {
		t.Fatalf("dx = %v, want spring-back to 0", s.dx)
	}
}
