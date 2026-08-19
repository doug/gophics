//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// Using a feature for the first time mid-session must not compile anything.
//
// Startup cost and mid-session cost are different problems. A shader compiled
// during init delays first paint once; the same shader compiled the first time
// the user scrolls to a screen that uses clipping is a dropped frame in the
// middle of an interaction, which is the one people actually see. This asserts
// the second case does not happen: after a plain frame has rendered, the first
// clipped frame and the first overlapping frame must create no GPU resources.
//
// TestSteadyStateResourceChurnIsZero covers repeated frames of a scene already
// seen. This covers the transition to a scene shape never seen before, which is
// where lazy compilation would hide.
func TestFirstUseOfAFeatureCostsNothing(t *testing.T) {
	h := newOffscreenHarness(t, 512)
	defer h.close()

	wgpu.EnableStats(true)
	defer wgpu.EnableStats(false)

	// A plain fill frame first, so everything eagerly built is already built.
	if err := h.renderOnce(gg.PipelineModeRenderPass, harnessScene(512, 8)); err != nil {
		t.Fatalf("plain frame: %v", err)
	}

	newFeatures := []struct {
		name   string
		shapes []harnessShape
	}{
		{"first clipped frame", harnessClipScene(512, 8)},
		{"first overlapping frame", harnessOverlapScene(512, 8)},
		{"first single-clip frame", harnessOneClipScene(512, 8)},
	}

	for _, f := range newFeatures {
		t.Run(f.name, func(t *testing.T) {
			before := wgpu.Snapshot()
			if err := h.renderOnce(gg.PipelineModeRenderPass, f.shapes); err != nil {
				t.Fatalf("render: %v", err)
			}
			delta := wgpu.Snapshot().Sub(before)

			if n := delta.Total(); n != 0 {
				t.Errorf("%s created %d GPU resources — a mid-session hitch", f.name, n)
				for _, r := range delta.Kinds {
					if r.Count > 0 {
						t.Errorf("  %-16s %d, %v", r.Kind, r.Count, r.Duration())
					}
				}
				for _, l := range delta.SortedLabels() {
					t.Errorf("  %s x%d", l.Label, l.Count)
				}
			}
		})
	}
}

// Records what init costs versus what the first frame costs, so the two are
// never conflated again. Init and first-frame are both "startup", but only the
// first-frame half scales with what the app draws.
func TestReportStartupCostSplit(t *testing.T) {
	wgpu.EnableStats(true)
	defer wgpu.EnableStats(false)
	wgpu.ResetStats()

	h := newOffscreenHarness(t, 512)
	defer h.close()
	initCost := wgpu.Snapshot()

	if err := h.renderOnce(gg.PipelineModeRenderPass, harnessScene(512, 8)); err != nil {
		t.Fatalf("first frame: %v", err)
	}
	frameCost := wgpu.Snapshot().Sub(initCost)

	logPhase(t, "device + context init", initCost)
	logPhase(t, "first rendered frame", frameCost)
}

func logPhase(t *testing.T, name string, s wgpu.StatsSnapshot) {
	t.Helper()
	var total int64
	for _, r := range s.Kinds {
		total += r.Nanos
	}
	t.Logf("%s: %d resources, %v", name, s.Total(), durationOf(total))
	for _, r := range s.Kinds {
		if r.Count > 0 {
			t.Logf("    %-17s %2d  %v", r.Kind, r.Count, r.Duration())
		}
	}
}

func durationOf(nanos int64) interface{ String() string } {
	return wgpu.ResourceStat{Nanos: nanos}.Duration()
}
