//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/internal/gpu/tilecompute"
)

// TestComputeStagesMatchCPU compares the GPU pipeline against its CPU port one
// stage at a time, rather than only at the finished image.
//
// This is the test that would have collapsed the M11 investigation from a day
// into a run. The symptom then was a wrong picture, and every structural check
// passed because none of them was wrong — buffer sizes, dispatch shapes,
// strides and struct layouts were all correct. What was actually true was that
// two stages disagreed: coarse reserved 16 segment slots and path_tiling filled
// 8. Diffing these buffers says that in one line.
//
// Restricted to single-path scenes because tilecompute's Rasterize takes one
// path's lines; the multi-path scenes are covered at image level by
// TestVelloComputeGolden. The stage that broke was per-path anyway.
func TestComputeStagesMatchCPU(t *testing.T) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()
	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	for _, tc := range computeGoldenTests() {
		if len(tc.Paths) != 1 {
			continue
		}
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			gpu, err := accel.DebugComputeStages(tc.Width, tc.Height, tc.BgColor, tc.Paths)
			if err != nil {
				t.Fatalf("DebugComputeStages: %v", err)
			}

			var cpu tilecompute.StageCapture
			tilecompute.NewRasterizer(tc.Width, tc.Height).
				RasterizeCapturing(tc.Paths[0].Lines, tc.Paths[0].FillRule, &cpu)

			// A scene that produces no work would make every comparison
			// vacuously true, which is the failure mode this whole exercise
			// exists to avoid.
			if cpu.Bump.Segments == 0 {
				t.Fatal("CPU port produced no segments — the fixture does not exercise the pipeline")
			}

			diffs := DiffComputeStages(gpu, &cpu)
			for i, d := range diffs {
				// Later stages consume earlier ones, so only the first is a
				// root cause; the rest are consequences.
				if i == 0 {
					t.Errorf("first divergence — %s", d)
					continue
				}
				t.Logf("downstream: %s", d)
			}
		})
	}
}
