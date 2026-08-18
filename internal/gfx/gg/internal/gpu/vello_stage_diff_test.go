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
// Covers multi-path scenes too. The GPU packs every path into one Tiles array
// with a base per path, while the CPU port rasterises each path independently
// from index zero, so the comparison goes through each path's tile-space bbox —
// and checks that bbox agrees first, since tiles compared through disagreeing
// bboxes would agree about nothing.
func TestComputeStagesMatchCPU(t *testing.T) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		requireGPU(t, err, "GPU not available")
	}
	defer accel.Close()
	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	for _, tc := range computeGoldenTests() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			gpu, err := accel.DebugComputeStages(tc.Width, tc.Height, tc.BgColor, tc.Paths)
			if err != nil {
				t.Fatalf("DebugComputeStages: %v", err)
			}

			cpus := captureCPUStages(tc.Width, tc.Height, tc.Paths)

			// A scene that produces no work would make every comparison
			// vacuously true, which is the failure mode this whole exercise
			// exists to avoid.
			total := uint32(0)
			for i := range cpus {
				total += cpus[i].Bump.Segments
			}
			if total == 0 {
				t.Fatal("CPU port produced no segments — the fixture does not exercise the pipeline")
			}

			diffs := DiffComputeStages(gpu, cpus)
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

// captureCPUStages runs the CPU port over each path, mirroring how the GPU
// pipeline handles a multi-path scene.
func captureCPUStages(w, h int, paths []tilecompute.PathDef) []tilecompute.StageCapture {
	caps := make([]tilecompute.StageCapture, len(paths))
	for i, pd := range paths {
		tilecompute.NewRasterizer(w, h).RasterizeCapturing(pd.Lines, pd.FillRule, &caps[i])
	}
	return caps
}
