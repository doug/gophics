//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/internal/renderref"
	"github.com/doug/gophics/widget"
)

// F1: does the renderer create GPU objects on every frame, or only at first use?
//
// design/rendering-pipeline.md's headline finding is that tier 2b destroys and
// recreates six GPU objects per path per changed frame — four buffers and two
// bind groups from createRenderBuffers — and that this never amortizes the way
// a compiled pipeline or an allocated texture does. Until buffers and bind
// groups were counted, that claim was unfalsifiable: the repo's instruments
// could see textures and pipelines, which are exactly the two kinds the finding
// does *not* accuse.
//
// This is the shape of the measurement, not a threshold to pass. It reports
// per-frame creation after warm-up for each corpus scene, so the numbers land
// in CI output where a later phase's before/after can be read off directly.
// It fails only on the one thing that would make the plan's premise wrong: if
// steady-state creation is zero everywhere, F1 does not exist and Phase C
// should not be written.
func TestF1SteadyStateObjectChurn(t *testing.T) {
	scenes := []struct {
		name string
		root widget.Widget
	}{
		{"mixed", renderref.Scene()},
		{"ui-screen", renderref.UIScreen()},
		{"stroke-heavy", renderref.StrokeHeavy()},
		{"curve-heavy", renderref.CurveHeavy()},
		{"text-heavy", renderref.TextHeavy()},
	}

	var anyChurn bool
	for _, sc := range scenes {
		h, err := NewHeadless(sc.root, Config{Size: renderref.SceneSize, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		// Warm up: the first frames compile pipelines and allocate the atlas and
		// attachments. Those are first-use costs and are not what F1 is about,
		// so they must be outside the measurement or they would swamp it.
		for range 3 {
			if h.RenderGPU() == nil {
				t.Skip("no GPU adapter available")
			}
		}

		const frames = 10
		before := wgpu.DeviceStats()
		for range frames {
			h.RenderGPU()
		}
		made := wgpu.DeviceStats().Sub(before)

		perFrame := func(n uint64) float64 { return float64(n) / frames }
		t.Logf("%-13s per frame: %6.1f buffers  %6.1f bind groups  %5.1f textures  %5.1f pipelines",
			sc.name, perFrame(made.Buffers), perFrame(made.BindGroups),
			perFrame(made.Textures), perFrame(made.Pipelines))
		if made.Buffers+made.BindGroups > 0 {
			anyChurn = true
		}
	}

	if !anyChurn {
		t.Error("no scene created a buffer or bind group in steady state: F1 does not " +
			"exist as described, and Phase C of design/rendering-pipeline.md is " +
			"aimed at nothing — re-scope before writing it")
	}
}
