package trace

import (
	"math"
	"testing"
)

// The frame that carries the release must not move the content more than a
// finger frame did. It used to: the finger's last delta and the first momentum
// step landed in the same frame, so every release began with a ~1.9× jump —
// 38px between neighbours of 20 and 17 in the first trace this harness ever
// produced. A native scroll view starts momentum at the release instant and
// integrates forward only.
//
// This is the harness's first regression test, and the shape is the point: a
// physics property stated as a bound on a measured curve, not as an assertion
// about internal state.
func TestReleaseFrameDoesNotKick(t *testing.T) {
	const hz, fingerV = 120.0, -2400.0
	tr, err := Replay(SyntheticFlick(fingerV, 0.1, hz), ReplayOptions{Hz: hz})
	if err != nil {
		t.Fatal(err)
	}
	fingerStep := math.Abs(fingerV) / hz // px per frame while the finger moved

	maxStep, at := 0.0, 0.0
	for i := 1; i < len(tr.Offset); i++ {
		if tr.Offset[i].T < tr.ReleaseT-1e-9 {
			continue
		}
		if d := tr.Offset[i].V - tr.Offset[i-1].V; d > maxStep {
			maxStep, at = d, tr.Offset[i].T
		}
	}
	// A little over a finger step is allowed: the velocity estimate can sit a
	// hair above the finger's speed as it converges. Nearly double is not.
	if maxStep > fingerStep*1.15 {
		t.Errorf("frame at t=%.4f moved %.2fpx; the finger moved %.2fpx per frame — the release frame double-counts", at, maxStep, fingerStep)
	}
}

// Momentum must still happen. A fix that stopped the kick by stopping the
// fling would pass the test above and fail the user.
func TestReleaseStillFlings(t *testing.T) {
	tr, _ := Replay(SyntheticFlick(-2400, 0.1, 120), ReplayOptions{Hz: 120})
	m := tr.Compute()
	if m.MomentumDist < 500 {
		t.Errorf("momentum distance %.0fpx after a firm flick; the fling did not run", m.MomentumDist)
	}
	if math.Abs(m.Tau-0.5) > 0.05 {
		t.Errorf("tau %.3f; the fix changed the decay, which it must not", m.Tau)
	}
}
