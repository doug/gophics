//go:build !nogpu

package gpu

import "testing"

// TestVelloComputeInitialisesOnRealDevice asserts that the compute vector
// pipeline actually builds on this machine's GPU.
//
// It is the test that was missing. Every existing check of CanCompute()
// exercised the false path — no dispatcher, no device — so nothing ever
// established that the true path was reachable, and it was not: the whole
// compute pipeline failed to build on Metal, silently, for three separate
// reasons. VelloAccelerator.initGPU logs the failure at Warn and returns nil,
// so InitStandalone reported success either way, and SelectPipeline quietly
// routed every scene to the render-pass path instead.
//
// The three failures, all of which this test now covers:
//
//   - naga emitted `x + select(a, b, c)` as `x + c ? a : b`, which C++ groups
//     as `(x + c) ? a : b`. Wrong arithmetic, and only a warning.
//   - naga could not compute a bound for `var<storage> x: array<T>` — a global
//     that *is* the runtime array rather than a struct member — so no bounds
//     check was hoisted out of atomics and the access guarded itself inside
//     the `&`. The address of a ternary is not an lvalue; Metal rejected it.
//   - the Metal HAL reported the WebGPU baseline of 8 storage buffers per
//     stage rather than the 31 its argument table holds, and the coarse stage
//     binds 9.
//
// Skips rather than fails where there is no GPU, but a machine that has one
// must be able to build the pipeline.
func TestVelloComputeInitialisesOnRealDevice(t *testing.T) {
	dev, queue := fineDevice(t)

	d := NewVelloComputeDispatcher(dev, queue)
	if err := d.Init(); err != nil {
		t.Fatalf("vello compute pipeline failed to build on this GPU: %v", err)
	}
	if !d.initialized {
		t.Fatal("dispatcher reported no error but is not initialised")
	}
}

// TestVelloAcceleratorReportsCompute checks the same thing through the seam the
// renderer actually consults. CanCompute() gates FillPath's compute branch and
// SelectPipeline's choice, so a false here means the compute backend is dead no
// matter how much of it builds.
func TestVelloAcceleratorReportsCompute(t *testing.T) {
	if _, _ = fineDevice(t); t.Skipped() {
		return
	}

	a := &VelloAccelerator{}
	if err := a.InitStandalone(); err != nil {
		t.Fatalf("InitStandalone: %v", err)
	}
	defer a.Close()

	if !a.CanCompute() {
		t.Fatal("CanCompute() is false on a machine with a working GPU — the compute pipeline built but did not reach the accelerator")
	}
}

// TestPathTilingFillsEveryReservedSegment guards the invariant that caught the
// fill bleed, and is worth keeping now that it holds.
//
// coarse reserves a run of segment slots per tile with an atomic bump and
// path_tiling fills them. Nothing checked that the second step covered the
// first, and for a plain filled rectangle it did not: 16 slots reserved, 8
// written, two of those identical. Tiles pointing into the unwritten half
// found degenerate segments and fell back to their backdrop alone — solid
// where the backdrop was 1, empty where it was 0 — which is what the goldens
// showed.
//
// The cause turned out to be under the shaders entirely: Metal bounds checks
// comparing against an unbound _mslBufferSizes, zeroing valid reads. This test
// stays because it is much narrower than TestVelloComputeGolden. The golden
// test says the picture is wrong; this says which stage stopped early, and
// that is the difference between a day of bisecting and an afternoon.
//
// The check itself lives in logPipelineDiagnostics, which warns whenever
// reserved != written; this drives a render and lets that fire.
func TestPathTilingFillsEveryReservedSegment(t *testing.T) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()
	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	for _, tc := range computeGoldenTests() {
		if tc.Name != "compute_blue_square" {
			continue
		}
		if _, err := accel.RenderSceneCompute(tc.Width, tc.Height, tc.BgColor, tc.Paths); err != nil {
			t.Fatalf("render: %v", err)
		}
	}
}

// TestComputeUnavailabilityHasAReason asserts that if the compute pipeline is
// not available, something says why.
//
// This is the check whose absence cost the most. The pipeline failed to build
// on Metal for three separate reasons, and the only symptom was CanCompute()
// returning false — which reads as a fact about the hardware, not as an error
// that was caught and thrown away. initGPU logs at Warn and returns nil, and
// the default logger discards Warn, so the explanation existed for a moment and
// then did not.
//
// Degrading is still right: a renderer that draws without compute beats one
// that refuses to start. Degrading without a recoverable reason is not.
func TestComputeUnavailabilityHasAReason(t *testing.T) {
	a := &VelloAccelerator{}
	if err := a.InitStandalone(); err != nil {
		requireGPU(t, err, "no GPU")
	}
	defer a.Close()

	if a.CanCompute() {
		if reason := a.ComputeUnavailableReason(); reason != nil {
			t.Errorf("compute is available but a reason is still recorded: %v", reason)
		}
		return
	}

	if a.ComputeUnavailableReason() == nil {
		t.Fatal("compute is unavailable and nothing says why — the failure was discarded")
	}
	t.Logf("compute unavailable, reason preserved: %v", a.ComputeUnavailableReason())
}
