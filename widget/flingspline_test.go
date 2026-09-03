package widget

import (
	"math"
	"testing"
)

// The spline table is Android's: monotonic, starting at 0 and ending at 1, and
// front-loaded — more than half the distance is covered in the first third
// of the duration, which is the whole reason it is not an exponential.
func TestSplineTableShape(t *testing.T) {
	// The first entry is ~2e-5, not 0: the platform's bisection stops at
	// 1e-5 and so does this one, and splineAt guards t<=0 anyway.
	if splinePosition[0] > 1e-4 || splinePosition[splineSamples] != 1 {
		t.Fatalf("table runs %v..%v, want ~0..1", splinePosition[0], splinePosition[splineSamples])
	}
	for i := 1; i <= splineSamples; i++ {
		if splinePosition[i] < splinePosition[i-1] {
			t.Fatalf("table is not monotonic at %d: %v < %v", i, splinePosition[i], splinePosition[i-1])
		}
	}
	if third := splineAt(1.0 / 3); third < 0.55 {
		t.Errorf("a third of the way through, %.2f of the distance is covered; Android's curve is front-loaded (>0.55)", third)
	}
}

// Duration and distance follow OverScroller's formulas. The expected values
// are those formulas evaluated by hand for one release speed; the test pins
// that the code and the platform's arithmetic agree.
func TestSplineFlingMatchesOverScroller(t *testing.T) {
	const v, friction = 4000.0, 0.015
	dur, dist := splineFling(v, friction)
	// decel = ln(0.35·4000 / (0.015·51890.4)) = ln(1.7987) = 0.5871
	// dur   = e^(0.5871/1.3582)               = 1.541s
	// dist  = 778.36 · e^(2.3582/1.3582·0.5871) = 2157dp
	if math.Abs(dur-1.541) > 0.01 {
		t.Errorf("duration %.3fs, want 1.541s", dur)
	}
	if math.Abs(dist-2157)/2157 > 0.01 {
		t.Errorf("distance %.0fdp, want 2157dp", dist)
	}
	// Faster flicks go further and take longer, both logarithmically.
	d2, s2 := splineFling(2*v, friction)
	if d2 <= dur || s2 <= dist {
		t.Errorf("doubling speed gave dur %.3f dist %.0f, want both larger than %.3f/%.0f", d2, s2, dur, dist)
	}
	// Distance goes as v^(DR/(DR−1)) = v^1.74: doubling the speed gives 3.3×
	// the travel — super-linear, sub-quadratic, and nothing like the
	// exponential's linear v0·τ.
	if r := s2 / dist; r < 3.2 || r > 3.5 {
		t.Errorf("doubling speed gave %.2f× the distance, want ~3.34× (v^1.74)", r)
	}
}
