//go:build !nogpu

package gpu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// Steady-state GPU resource churn must be zero.
//
// A frame that allocates GPU resources and a frame that reuses them are
// indistinguishable in a wall-clock average -- the allocation shows up as an
// occasional hitch, which a mean hides and a benchmark's warmup discards. This
// asserts on the thing directly: after the first frame has built what it needs,
// later frames must create nothing.
//
// This is the check that would have caught the per-draw buffer allocation in
// the depth-clip path, which was found by reading code rather than by any test.
func TestSteadyStateResourceChurnIsZero(t *testing.T) {
	const warmup, measured = 3, 20

	h := newOffscreenHarness(t, 512)
	defer h.close()

	scenes := map[string][]harnessShape{
		"plain":   harnessScene(512, 64),
		"clipped": harnessClipScene(512, 64),
		"overlap": harnessOverlapScene(512, 64),
	}

	wgpu.EnableStats(true)
	defer wgpu.EnableStats(false)

	for name, shapes := range scenes {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < warmup; i++ {
				if err := h.renderOnce(gg.PipelineModeRenderPass, shapes); err != nil {
					t.Fatalf("warmup frame %d: %v", i, err)
				}
			}

			before := wgpu.Snapshot()
			for i := 0; i < measured; i++ {
				if err := h.renderOnce(gg.PipelineModeRenderPass, shapes); err != nil {
					t.Fatalf("measured frame %d: %v", i, err)
				}
			}
			delta := wgpu.Snapshot().Sub(before)

			if n := delta.Total(); n != 0 {
				t.Errorf("%d GPU resources created across %d steady-state frames (want 0)", n, measured)
				for _, r := range delta.Kinds {
					if r.Count > 0 {
						t.Errorf("  %-16s %d created, %v, %d bytes", r.Kind, r.Count, r.Duration(), r.Bytes)
					}
				}
				for _, l := range delta.SortedLabels() {
					t.Errorf("  label %-40s x%d", l.Label, l.Count)
				}
			}
		})
	}
}

// Records what a cold start costs and what steady state costs, as JSON for the
// dashboard. Writing the numbers down is what makes the next change checkable
// against them; a claim of "no slower" needs a previous run to mean anything.
//
// Set GOPHICS_STATS_OUT to a path to keep the file.
func TestWriteGPUStatsReport(t *testing.T) {
	h := newOffscreenHarness(t, 512)
	defer h.close()

	shapes := harnessScene(512, 64)

	wgpu.EnableStats(true)
	defer wgpu.EnableStats(false)
	wgpu.ResetStats()

	if err := h.renderOnce(gg.PipelineModeRenderPass, shapes); err != nil {
		t.Fatalf("cold frame: %v", err)
	}
	cold := wgpu.Snapshot()

	for i := 0; i < 30; i++ {
		if err := h.renderOnce(gg.PipelineModeRenderPass, shapes); err != nil {
			t.Fatalf("warm frame %d: %v", i, err)
		}
	}
	steady := wgpu.Snapshot().Sub(cold)

	report := struct {
		Cold   wgpu.StatsSnapshot `json:"cold_first_frame"`
		Steady wgpu.StatsSnapshot `json:"steady_state_30_frames"`
	}{cold, steady}

	buf, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out := os.Getenv("GOPHICS_STATS_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "gpu_stats.json")
	}
	if err := os.WriteFile(out, buf, 0o600); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}

	t.Logf("cold first frame: %d resources", cold.Total())
	for _, r := range cold.Kinds {
		if r.Count > 0 {
			t.Logf("  %-16s %3d  %v", r.Kind, r.Count, r.Duration())
		}
	}
	t.Logf("steady state (30 frames): %d resources", steady.Total())
	t.Logf("report written to %s", out)
}
