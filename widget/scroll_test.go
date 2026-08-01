package widget

import (
	"math"
	"testing"
)

// The rubber-band mapping is the heart of the native-feel overscroll: the
// first pixels move nearly freely, then resistance grows and the displacement
// asymptotes toward the viewport extent so it never runs away.
func TestRubberBandShape(t *testing.T) {
	const ext = 300

	if got := rubberBand(0, ext); got != 0 {
		t.Fatalf("rubberBand(0) = %v, want 0", got)
	}

	// Sign is preserved.
	if rubberBand(50, ext) <= 0 || rubberBand(-50, ext) >= 0 {
		t.Fatalf("rubberBand does not preserve sign")
	}

	// Bounded below the extent and monotonic in distance.
	prev := float32(0)
	for d := float32(1); d < 5000; d *= 1.5 {
		v := rubberBand(d, ext)
		if v >= ext {
			t.Fatalf("rubberBand(%v) = %v not bounded by extent %v", d, v, ext)
		}
		if v <= prev {
			t.Fatalf("rubberBand not monotonic at d=%v (%v <= %v)", d, v, prev)
		}
		prev = v
	}

	// Near zero the response starts around the resistance factor (~0.55): a
	// small pull barely resists, matching the initial 1:1-ish feel.
	small := rubberBand(1, ext)
	if small < 0.4 || small > 0.6 {
		t.Fatalf("initial slope = %v, want ~%v", small, rubberC)
	}
}

func TestRubberBandInverseRoundTrips(t *testing.T) {
	const ext = 300
	for _, d := range []float32{-800, -120, -10, 5, 30, 90, 250, 600} {
		disp := rubberBand(d, ext)
		back := inverseRubberBand(disp, ext)
		if math.Abs(float64(back-d)) > 0.5 {
			t.Fatalf("inverse(rubber(%v)) = %v, want %v", d, back, d)
		}
	}
}

// A degenerate (zero) extent must not divide by zero; it passes distance
// through unchanged.
func TestRubberBandZeroExtent(t *testing.T) {
	if got := rubberBand(42, 0); got != 42 {
		t.Fatalf("rubberBand(42, 0) = %v, want 42", got)
	}
	if got := inverseRubberBand(42, 0); got != 42 {
		t.Fatalf("inverseRubberBand(42, 0) = %v, want 42", got)
	}
}
