package trace

import "testing"

// A gesture with no movement is a tap, not a flick: replay must release
// immediately, record nothing moving, and Compute must return zeros rather
// than divide by the nothing it was given.
func TestReplayWithNoInputIsATap(t *testing.T) {
	tr, err := Replay(nil, ReplayOptions{Hz: 60})
	if err != nil {
		t.Fatal(err)
	}
	if tr.ReleaseT <= 0 {
		t.Error("no release recorded for an empty gesture")
	}
	m := tr.Compute()
	if m.TotalDist != 0 || m.MomentumDist != 0 || m.Tau != 0 {
		t.Errorf("a tap produced motion metrics: %+v", m)
	}
}

// Metrics on a trace too short to differentiate must be zeros, not a panic:
// a native twin that quit early writes exactly this.
func TestComputeOnTinyTraceIsZero(t *testing.T) {
	for _, n := range []int{0, 1, 2} {
		tr := &Trace{Hz: 60}
		for i := 0; i < n; i++ {
			tr.Offset = append(tr.Offset, Sample{T: float64(i) / 60, V: float64(i)})
		}
		if m := tr.Compute(); m != (Metrics{}) {
			t.Errorf("%d-sample trace gave %+v, want zeros", n, m)
		}
	}
}
