package anim

import (
	"math"
	"testing"
)

// A spring settles by overshooting, which is the whole reason to have one.
func TestSpringOvershootsThenSettles(t *testing.T) {
	if got := Spring(0); got != 0 {
		t.Errorf("Spring(0) = %v, want 0", got)
	}
	if got := Spring(1); got != 1 {
		t.Errorf("Spring(1) = %v, want 1", got)
	}

	// Somewhere in the middle it must go past its target — an ease-out never
	// does, and that difference is what the motion is for.
	var peak float32
	var peakAt float64
	for i := 1; i < 1000; i++ {
		x := float64(i) / 1000
		if v := Spring(float32(x)); v > peak {
			peak, peakAt = v, x
		}
	}
	if peak <= 1 {
		t.Errorf("peak %v at t=%.3f: the curve never overshoots, so it is not a spring", peak, peakAt)
	}
	// A few percent reads as elastic; a lot reads as a bouncing ball.
	if peak > 1.15 {
		t.Errorf("peak %v at t=%.3f overshoots too far for a settle", peak, peakAt)
	}

	// And it has to be done by the end, or the jump to exactly 1 shows.
	for _, x := range []float32{0.9, 0.95, 0.99} {
		if d := math.Abs(float64(Spring(x)) - 1); d > 0.02 {
			t.Errorf("Spring(%v) is %v from rest; it has not settled by the end", x, d)
		}
	}
}

// EaseOut is what a dismissal uses, and it must not overshoot: a card on its
// way off the screen cannot come back a few pixels first.
func TestEaseOutDoesNotOvershoot(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		x := float32(i) / 1000
		if v := EaseOut(x); v > 1.0001 {
			t.Fatalf("EaseOut(%v) = %v, past its target", x, v)
		}
	}
}
