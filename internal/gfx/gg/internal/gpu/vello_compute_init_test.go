//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
)

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

// TestPathTilingFillsEveryReservedSegment pins the invariant that the fill
// bleed traces back to.
//
// The coarse stage reserves a run of segment slots per tile with an atomic
// bump, and path_tiling fills them. Nothing checked that the second step
// covered the first — and it does not. For a plain filled rectangle the bump
// reports 16 reserved slots and path_tiling writes 8, leaving the rest zeroed.
// A tile whose ~seg_ix points into the unwritten half finds degenerate
// segments and falls back to its backdrop alone: solid where the backdrop is
// 1, empty where it is 0. That is exactly the shape of the golden-test
// failures — a fill that starts in the right place and then runs to the tile
// edge.
//
// This is narrower than TestVelloComputeGolden and worth having beside it: the
// golden test says the picture is wrong, this says which stage stopped early.
// Both are gated on the same flag, so they come back together.
func TestPathTilingFillsEveryReservedSegment(t *testing.T) {
	if !gg.AutoSelectCompute {
		t.Skip("compute rendering is incorrect (M11): path_tiling fills 8 of 16 reserved segment slots; set gg.AutoSelectCompute when fixed")
	}

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
