package trace

import (
	"math"
	"testing"

	"github.com/doug/gophics/shell"
)

// The harness's first number: gophics's own fling decays with the time
// constant its physics declares. flingFriction is 2.0/s, so tau is 0.5s; a
// measured value far from that means the harness is measuring something
// other than the fling — or the fling is not what its constant says.
func TestReplayMeasuresGophicsTau(t *testing.T) {
	input := SyntheticFlick(-2400, 0.1, 120) // a firm upward flick
	tr, err := Replay(input, ReplayOptions{Hz: 120})
	if err != nil {
		t.Fatal(err)
	}
	m := tr.Compute()

	if m.TotalDist <= 0 {
		t.Fatalf("an upward flick moved the content %v; the sign convention is broken", m.TotalDist)
	}
	if m.MomentumDist <= 0 {
		t.Fatal("no momentum after release: the fling did not start")
	}
	if math.Abs(m.Tau-0.5) > 0.05 {
		t.Errorf("measured tau %.3fs, want 0.5s (flingFriction 2.0); R²=%.3f", m.Tau, m.TauR2)
	}
	if m.TauR2 < 0.98 {
		t.Errorf("decay fit R² %.3f; gophics's fling is a pure exponential and should fit near 1", m.TauR2)
	}
	t.Logf("\n%s", m)
}

// Two runs of the same input give the same curve. Without this, a
// comparison against a native trace could never distinguish physics from
// noise in the harness itself.
func TestReplayIsDeterministic(t *testing.T) {
	input := SyntheticFlick(-1800, 0.08, 120)
	a, _ := Replay(input, ReplayOptions{Hz: 120})
	b, _ := Replay(input, ReplayOptions{Hz: 120})
	if len(a.Offset) != len(b.Offset) {
		t.Fatalf("runs produced %d and %d frames", len(a.Offset), len(b.Offset))
	}
	for i := range a.Offset {
		if a.Offset[i] != b.Offset[i] {
			t.Fatalf("frame %d differs: %v vs %v", i, a.Offset[i], b.Offset[i])
		}
	}
}

// The decay is a property of time, not of frame count: the same flick at
// 60Hz and 120Hz must settle in the same wall time. This is the refresh-rate
// independence the velocityTau comment promises, checked.
func TestReplayIsRefreshRateIndependent(t *testing.T) {
	in60 := SyntheticFlick(-2400, 0.1, 60)
	in120 := SyntheticFlick(-2400, 0.1, 120)
	a, _ := Replay(in60, ReplayOptions{Hz: 60})
	b, _ := Replay(in120, ReplayOptions{Hz: 120})
	ma, mb := a.Compute(), b.Compute()
	if r := ma.MomentumDist / mb.MomentumDist; r < 0.9 || r > 1.1 {
		t.Errorf("momentum distance 60Hz/120Hz = %.3f (%.0f vs %.0f px); should be ~1", r, ma.MomentumDist, mb.MomentumDist)
	}
	if math.Abs(ma.Tau-mb.Tau) > 0.05 {
		t.Errorf("tau differs by refresh rate: %.3f at 60Hz vs %.3f at 120Hz", ma.Tau, mb.Tau)
	}
}

func TestMetricsOnSyntheticExponential(t *testing.T) {
	// A hand-built pure exponential with tau 0.3 must measure as tau 0.3.
	tr := &Trace{Hz: 100, ReleaseT: 0}
	v0, tau := 1000.0, 0.3
	off := 0.0
	for i := 0; i <= 300; i++ {
		tt := float64(i) / 100
		tr.Offset = append(tr.Offset, Sample{T: tt, V: off})
		off += v0 * math.Exp(-tt/tau) / 100
	}
	m := tr.Compute()
	if math.Abs(m.Tau-tau) > 0.02 {
		t.Errorf("tau %.3f, want %.3f", m.Tau, tau)
	}
	if m.TauR2 < 0.999 {
		t.Errorf("R² %.4f on a pure exponential", m.TauR2)
	}
}

// Under Android's physics the same flick is a different curve: it ends on a
// known clock instead of tailing off, and its travel is the platform's
// formula applied to the velocity the finger handed over.
func TestReplayUnderAndroidPhysics(t *testing.T) {
	input := SyntheticFlick(-2400, 0.1, 120)
	tr, err := Replay(input, ReplayOptions{Hz: 120, Physics: shell.AndroidScrollPhysics()})
	if err != nil {
		t.Fatal(err)
	}
	m := tr.Compute()
	if m.MomentumDist <= 0 {
		t.Fatal("no momentum under the spline model")
	}
	// The spline stops dead at its duration rather than decaying under the
	// rest threshold, so it settles well inside the exponential's 2.3s.
	if m.SettleT > 1.6 {
		t.Errorf("settled in %.2fs; Android's curve for this flick ends near 1.2s", m.SettleT)
	}
	// And it is not an exponential — the fit should say so.
	if m.TauR2 > 0.98 {
		t.Errorf("R² %.3f: the spline fit an exponential too well to be one", m.TauR2)
	}
	// Offsets never go backwards.
	for i := 1; i < len(tr.Offset); i++ {
		if tr.Offset[i].V < tr.Offset[i-1].V-1e-6 {
			t.Fatalf("offset reversed at t=%.3f", tr.Offset[i].T)
		}
	}
	t.Logf("\n%s", m)
}
