//go:build gophics_gpu

package app

import (
	"os"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/renderref"
	"github.com/doug/gophics/widget"
)

// What does 4x MSAA cost?
//
// design/rendering-pipeline.md §3 argues that 4x MSAA exists solely to
// anti-alias tier 2b — every other tier brings its own coverage — and that
// every tier pays for it: 4x the color attachment, 4x the depth/stencil
// attachment, and a resolve per pass. §4.5(3) calls pricing that the highest
// information available without building timestamp queries, and Phase D is
// explicitly gated on the answer: if MSAA is cheap, the largest item in the
// plan is not worth its risk.
//
// Run both halves and compare:
//
//	go test -tags gophics_gpu -run TestMSAAAblation -v ./app/
//	GOGPU_NO_MSAA=1 go test -tags gophics_gpu -run TestMSAAAblation -v ./app/
//
// Wall-clock around RenderGPU, which ends in a readback and therefore waits for
// the GPU — coarse, but it measures execution rather than submission, which is
// the trap §2.5 describes. Times vary run to run; treat a difference under ~15%
// as noise, per §4.4's "counts gate, times report".
func TestMSAAAblation(t *testing.T) {
	mode := "4x MSAA"
	if os.Getenv("GOGPU_NO_MSAA") != "" {
		mode = "1x (ablated)"
	}
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
	for _, sc := range scenes {
		h, err := NewHeadless(sc.root, Config{Size: renderref.SceneSize, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		for range 5 { // warm up: pipelines, atlas, attachments
			if h.RenderGPU() == nil {
				t.Skip("no GPU adapter available")
			}
		}
		const frames = 40
		best := time.Duration(1 << 62) // best-of, to suppress scheduler noise
		var total time.Duration
		for range frames {
			t0 := time.Now()
			h.RenderGPU()
			d := time.Since(t0)
			total += d
			best = min(best, d)
		}
		t.Logf("%-11s %-13s mean %6.2f ms   best %6.2f ms",
			mode, sc.name,
			float64(total.Microseconds())/float64(frames)/1000,
			float64(best.Microseconds())/1000)
	}
}
