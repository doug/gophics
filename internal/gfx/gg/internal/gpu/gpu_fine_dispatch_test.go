//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/scene"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// fineDevice returns a real GPU device, skipping when the machine has none.
func fineDevice(t *testing.T) (*wgpu.Device, *wgpu.Queue) {
	t.Helper()
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("no GPU instance: %v", err)
	}
	ad, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	dev, err := ad.RequestDevice(&wgpu.DeviceDescriptor{Label: "gpu_fine_dispatch_test"})
	if err != nil {
		t.Skipf("no device: %v", err)
	}
	t.Cleanup(func() { dev.Release() })
	return dev, dev.Queue()
}

// fineRectScene builds the coarse output for an axis-aligned filled rectangle.
// Two vertical edges with opposite winding are the whole of a rectangle fill,
// and axis-aligned edges keep the expected coverage easy to reason about: the
// interior is solid, the boundary tiles are partial.
func fineRectScene(t *testing.T, w, h uint16) (*CoarseRasterizer, *SegmentList, []int32) {
	t.Helper()
	segments := NewSegmentList()
	segments.AddLine(8, 8, 8, 24, 1)
	segments.AddLine(24, 24, 24, 8, -1)

	coarse := NewCoarseRasterizer(w, h)
	coarse.Rasterize(segments)
	if len(coarse.Entries()) == 0 {
		t.Skip("coarse rasterizer produced no entries")
	}
	return coarse, segments, coarse.CalculateBackdrop()
}

// fineTriangleScene builds the coarse output for a filled triangle. Its sloped
// edges are the point: an axis-aligned rectangle only ever yields coverage of 0
// or 255, which compares the two implementations' bookkeeping but never their
// arithmetic. A diagonal drives the trapezoidal area computation through
// partial values, where a divergence between the shader and the Go
// transcription would actually show up.
func fineTriangleScene(t *testing.T, w, h uint16) (*CoarseRasterizer, *SegmentList, []int32) {
	t.Helper()
	segments := NewSegmentList()
	segments.AddLine(10, 6, 34, 30, 1)  // sloped left edge, downward
	segments.AddLine(40, 30, 40, 6, -1) // vertical right edge, upward

	coarse := NewCoarseRasterizer(w, h)
	coarse.Rasterize(segments)
	if len(coarse.Entries()) == 0 {
		t.Skip("coarse rasterizer produced no entries")
	}
	return coarse, segments, coarse.CalculateBackdrop()
}

// TestGPUFineMatchesCPU is the point of the whole dispatch path: the shader and
// the Go transcription of it must agree, pixel for pixel.
//
// It asserts on pixels, not on error returns. GPUFineRasterizer spent its first
// life compiling shaders, creating pipelines and then computing on the CPU —
// every test passed, because correct pixels came back from the wrong place. So
// this calls RasterizeGPU, which cannot fall back, and separately requires the
// result to contain ink: an all-zero buffer matches an all-zero reference, and
// "the GPU wrote nothing" must not be able to pass as agreement.
func TestGPUFineMatchesCPU(t *testing.T) {
	dev, queue := fineDevice(t)

	const w, h = 64, 64
	r, err := NewGPUFineRasterizer(dev, queue, w, h)
	if err != nil {
		t.Fatalf("NewGPUFineRasterizer: %v", err)
	}
	defer r.Destroy()

	scenes := []struct {
		name string
		// wantPartial marks a scene whose sloped edges must yield
		// anti-aliased values. Without it a fixture could quietly degenerate
		// to whole-pixel coverage and the trapezoidal area code — the part
		// most likely to diverge between the shader and Go — would go
		// uncompared while the test still passed.
		wantPartial bool
		build       func(*testing.T, uint16, uint16) (*CoarseRasterizer, *SegmentList, []int32)
	}{
		{"rect", false, fineRectScene},
		{"triangle", true, fineTriangleScene},
	}
	rules := []struct {
		name string
		fill scene.FillStyle
	}{
		{"nonzero", scene.FillNonZero},
		{"evenodd", scene.FillEvenOdd},
	}

	for _, sc := range scenes {
		for _, rule := range rules {
			t.Run(sc.name+"/"+rule.name, func(t *testing.T) {
				coarse, segments, backdrop := sc.build(t, w, h)
				compareFineCoverage(t, r, coarse, segments, backdrop, rule.fill, sc.wantPartial)
			})
		}
	}
}

// compareFineCoverage diffs the GPU dispatch against the CPU reference for one
// scene and fill rule.
func compareFineCoverage(
	t *testing.T,
	r *GPUFineRasterizer,
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
	fill scene.FillStyle,
	wantPartial bool,
) {
	t.Helper()

	want, err := r.RasterizeCPU(coarse, segments, backdrop, fill)
	if err != nil {
		t.Fatalf("RasterizeCPU: %v", err)
	}
	got, err := r.RasterizeGPU(coarse, segments, backdrop, fill)
	if err != nil {
		t.Fatalf("RasterizeGPU: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("coverage length: GPU %d, CPU %d", len(got), len(want))
	}

	if countInk(want) == 0 {
		t.Fatal("reference coverage is empty — the fixture draws nothing, so agreement would be meaningless")
	}
	if countInk(got) == 0 {
		t.Fatal("GPU produced no coverage at all — the dispatch ran but wrote nothing")
	}
	if wantPartial && countPartial(want) == 0 {
		t.Fatal("fixture produced no anti-aliased coverage, so the trapezoidal area path went uncompared")
	}

	// A one-level tolerance: the shader and the Go code do the same arithmetic,
	// but not necessarily with the same rounding, and the result is quantised
	// to 8 bits at the very end.
	const tol = 1
	var diffs, worst, worstAt int
	for i := range want {
		d := int(got[i]) - int(want[i])
		if d < 0 {
			d = -d
		}
		if d > tol {
			diffs++
			if d > worst {
				worst, worstAt = d, i
			}
		}
	}
	if diffs > 0 {
		t.Errorf("GPU and CPU coverage disagree on %d of %d samples (worst %d at index %d: GPU %d, CPU %d; tolerance %d)",
			diffs, len(want), worst, worstAt, got[worstAt], want[worstAt], tol)
	}
}

// countPartial reports how many coverage samples are neither empty nor full.
func countPartial(coverage []uint8) int {
	n := 0
	for _, c := range coverage {
		if c != 0 && c != 255 {
			n++
		}
	}
	return n
}

// countInk reports how many coverage samples are non-zero.
func countInk(coverage []uint8) int {
	n := 0
	for _, c := range coverage {
		if c != 0 {
			n++
		}
	}
	return n
}

// TestFineTileOrderIsDeterministic pins the fix for a defect that predates the
// GPU dispatch: buildTileData grouped tiles in a map and then ranged over it,
// so the same scene produced its tiles — and therefore its coverage buffer — in
// a different order on every call. Nothing downstream could diff two runs or
// cache a tile, and it is why the GPU and CPU appeared to disagree here when
// they were in fact computing the same pixels in different slots.
func TestFineTileOrderIsDeterministic(t *testing.T) {
	segments := NewSegmentList()
	segments.AddLine(8, 8, 8, 24, 1)
	segments.AddLine(24, 24, 24, 8, -1)

	coarse := NewCoarseRasterizer(64, 64)
	coarse.Rasterize(segments)
	if len(coarse.Entries()) == 0 {
		t.Skip("coarse rasterizer produced no entries")
	}
	backdrop := coarse.CalculateBackdrop()

	r := &GPUFineRasterizer{}
	first, _ := r.buildTileData(coarse, segments, backdrop)
	if len(first) < 2 {
		t.Skipf("need at least two tiles to detect reordering, got %d", len(first))
	}

	// Several passes: map iteration order varies per range statement, so a
	// single repeat can coincide with the first by chance.
	for pass := 0; pass < 8; pass++ {
		again, _ := r.buildTileData(coarse, segments, backdrop)
		if len(again) != len(first) {
			t.Fatalf("pass %d: tile count changed: %d then %d", pass, len(first), len(again))
		}
		for i := range first {
			if again[i] != first[i] {
				t.Fatalf("pass %d: tile %d changed: %+v then %+v", pass, i, first[i], again[i])
			}
		}
	}

	// And the order is scanline order, which is what makes it predictable
	// rather than merely stable.
	for i := 1; i < len(first); i++ {
		prev, cur := first[i-1], first[i]
		if cur.TileY < prev.TileY || (cur.TileY == prev.TileY && cur.TileX <= prev.TileX) {
			t.Errorf("tiles not in scanline order at %d: (%d,%d) then (%d,%d)",
				i, prev.TileX, prev.TileY, cur.TileX, cur.TileY)
		}
	}
}

// TestGPUFineDispatchEncodesShaderLayout pins the struct sizes the encoders
// write against the layouts fine.wgsl declares. A field added on the Go side
// without one in the shader shifts every element after it, which shows up as
// subtly wrong pixels rather than an error.
func TestGPUFineDispatchEncodesShaderLayout(t *testing.T) {
	if got := len(encodeSegments(make([]GPUSegment, 3))); got != 3*32 {
		t.Errorf("segment stride: got %d bytes for 3, want %d", got, 3*32)
	}
	if got := len(encodeTileSegRefs(make([]GPUTileSegmentRef, 3))); got != 3*16 {
		t.Errorf("tile-segment-ref stride: got %d bytes for 3, want %d", got, 3*16)
	}
	if got := len(encodeTileInfos(make([]GPUTileInfo, 3))); got != 3*32 {
		t.Errorf("tile-info stride: got %d bytes for 3, want %d", got, 3*32)
	}
	if got := len(encodeFineConfig(64, 64, 1, scene.FillNonZero)); got != 32 {
		t.Errorf("config size: got %d bytes, want 32", got)
	}
}

// TestFineFillRuleCode pins the shader's fill-rule encoding.
func TestFineFillRuleCode(t *testing.T) {
	if got := fineFillRuleCode(scene.FillNonZero); got != 0 {
		t.Errorf("nonzero: got %d, want 0", got)
	}
	if got := fineFillRuleCode(scene.FillEvenOdd); got != 1 {
		t.Errorf("evenodd: got %d, want 1", got)
	}
}
