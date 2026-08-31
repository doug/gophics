package gpu

import "testing"

// strategyNoMSAA has to be reachable on hardware that supports MSAA.
//
// It existed and could not be selected: detectStrategy derives the strategy
// from sampleCount, sampleCount comes from probing the device, and a device
// that supports 4x always got 4x. So the MSAA ablation — the highest-information
// measurement available without building timestamp queries — could not be run
// on any machine where the answer was interesting.
//
// The override returns before the device is touched, which is why this can pass
// a nil device: if that ordering is ever reversed the test panics rather than
// silently probing.
func TestNoMSAAOverrideIsReachable(t *testing.T) {
	t.Setenv("GOGPU_NO_MSAA", "1")
	if got := resolveSampleCount(nil); got != 1 {
		t.Errorf("resolveSampleCount with GOGPU_NO_MSAA = %d, want 1", got)
	}
	// And that 1x actually selects the strategy, so the ablation changes the
	// render path rather than only the number.
	sh := &GPUShared{sampleCount: 1}
	if got := sh.detectStrategy(); got != strategyNoMSAA {
		t.Errorf("detectStrategy with sampleCount=1 = %v, want strategyNoMSAA", got)
	}
}
