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
