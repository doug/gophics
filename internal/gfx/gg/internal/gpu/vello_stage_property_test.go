//go:build !nogpu

package gpu

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/internal/gpu/tilecompute"
)

// randomPolygon builds a closed non-self-intersecting polygon by walking angles
// in order at varying radii. Star-shaped rather than arbitrary, because an
// arbitrary point cloud gives self-intersecting outlines whose fill depends on
// the fill rule in ways that make a disagreement hard to attribute — and the
// point here is to stress tiling and binning, not winding semantics.
func randomPolygon(rng *rand.Rand, cx, cy, rMin, rMax float32, n int) []tilecompute.LineSoup {
	pts := make([][2]float32, n)
	for i := 0; i < n; i++ {
		ang := 2 * math.Pi * float64(i) / float64(n)
		r := rMin + rng.Float32()*(rMax-rMin)
		pts[i] = [2]float32{
			cx + r*float32(math.Cos(ang)),
			cy + r*float32(math.Sin(ang)),
		}
	}
	lines := make([]tilecompute.LineSoup, n)
	for i := 0; i < n; i++ {
		lines[i] = tilecompute.LineSoup{
			PathIx: 0,
			P0:     pts[i],
			P1:     pts[(i+1)%n],
		}
	}
	return lines
}

// TestComputeStagesProperty feeds generated scenes through both pipelines and
// checks they agree stage by stage.
//
// The golden scenes are seven hand-picked cases that all pass; a differ fed
// only those confirms what is already known. Generated geometry turns the same
// tool into a search: odd tile alignments, paths that graze a tile boundary,
// counts that straddle a workgroup size — the shapes nobody thinks to write by
// hand, and exactly where a tiling pipeline goes wrong.
//
// Deterministic by seed so any failure names the case that produced it.
func TestComputeStagesProperty(t *testing.T) {
	if testing.Short() {
		t.Skip("generated-scene sweep skipped in -short")
	}

	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()
	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	bg := [4]uint8{255, 255, 255, 255}
	sizes := []int{64, 100, 128, 200, 256}
	fills := []tilecompute.FillRule{tilecompute.FillRuleNonZero, tilecompute.FillRuleEvenOdd}

	// 400 runs in about a second. Raising it is the cheapest way to search
	// harder: at 800 the sweep still finishes in under three seconds, and the
	// generator is seeded so anything it finds reproduces exactly.
	const cases = 400
	rng := rand.New(rand.NewSource(20260818))

	checked := 0
	for i := 0; i < cases; i++ {
		size := sizes[rng.Intn(len(sizes))]
		fill := fills[rng.Intn(len(fills))]
		verts := 3 + rng.Intn(9)
		half := float32(size) / 2
		// Radii deliberately reach past the canvas sometimes, so paths clip at
		// the edge — clipping is where tile bounds get interesting.
		rMax := half * (0.4 + rng.Float32()*0.9)
		rMin := rMax * (0.2 + rng.Float32()*0.6)
		cx := half + (rng.Float32()-0.5)*float32(size)*0.25
		cy := half + (rng.Float32()-0.5)*float32(size)*0.25

		// Most cases are a single path; a third carry two or three. The GPU
		// packs every path into one tile array with a base per path while the
		// CPU port starts each at zero, so multi-path scenes are the only ones
		// that exercise those bases and bboxes at all.
		nPaths := 1
		if rng.Intn(3) == 0 {
			nPaths = 2 + rng.Intn(2)
		}
		paths := make([]tilecompute.PathDef, nPaths)
		for p := range paths {
			pcx := half + (rng.Float32()-0.5)*float32(size)*0.5
			pcy := half + (rng.Float32()-0.5)*float32(size)*0.5
			pr := rMax * (0.3 + rng.Float32()*0.7)
			paths[p] = tilecompute.PathDef{
				Lines:    randomPolygon(rng, pcx, pcy, pr*0.4, pr, 3+rng.Intn(7)),
				Color:    [4]uint8{uint8(p * 90), 0, 255, 255},
				FillRule: fill,
			}
		}
		if nPaths == 1 {
			paths[0].Lines = randomPolygon(rng, cx, cy, rMin, rMax, verts)
		}

		name := fmt.Sprintf("case%03d_size%d_paths%d_verts%d_fill%d", i, size, nPaths, verts, fill)
		t.Run(name, func(t *testing.T) {
			gpu, err := accel.DebugComputeStages(size, size, bg, paths)
			if err != nil {
				t.Fatalf("DebugComputeStages: %v", err)
			}

			cpus := captureCPUStages(size, size, paths)
			total := uint32(0)
			for i := range cpus {
				total += cpus[i].Bump.Segments
			}
			if total == 0 {
				t.Skip("degenerate scene produced no segments")
			}

			diffs := DiffComputeStages(gpu, cpus)
			if len(diffs) == 0 {
				return
			}

			// A stage disagreement only matters if it reaches the picture.
			// Segments that graze a tile boundary can land either side of it
			// depending on rounding, which changes the buffers without
			// changing a pixel; that is a tolerance question, not a bug. So
			// the image decides the severity and the stage diff says where to
			// look.
			pixels := comparePipelineImages(t, accel, size, bg, paths)

			report := t.Errorf
			if pixels == 0 {
				report = t.Logf
			}
			for i, d := range diffs {
				if i == 0 {
					report("stage divergence (%d pixels differ) — %s\n  centre (%.2f,%.2f) radii %.2f..%.2f",
						pixels, d, cx, cy, rMin, rMax)
					continue
				}
				t.Logf("downstream: %s", d)
			}
		})
		checked++
	}
	t.Logf("compared %d generated scenes stage by stage", checked)
}

// comparePipelineImages renders the scene through both pipelines and returns
// the number of differing pixels.
func comparePipelineImages(t *testing.T, accel *VelloAccelerator, size int, bg [4]uint8, paths []tilecompute.PathDef) int {
	t.Helper()
	gpuImg, err := accel.RenderSceneCompute(size, size, bg, paths)
	if err != nil {
		t.Fatalf("RenderSceneCompute: %v", err)
	}
	cpuImg := tilecompute.NewRasterizer(size, size).RasterizeScenePTCL(bg, paths)

	n := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			cr, cg, cb, _ := cpuImg.At(x, y).RGBA()
			gr, gg2, gb, _ := gpuImg.At(x, y).RGBA()
			if cr>>8 != gr>>8 || cg>>8 != gg2>>8 || cb>>8 != gb>>8 {
				n++
			}
		}
	}
	return n
}
