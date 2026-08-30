//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/renderref"
	"github.com/doug/gophics/widget"
)

// GPU==CPU on every corpus scene, not just the reference one.
//
// The performance corpus was added to internal/renderref on the theory that
// putting the scenes beside the reference scene would let the correctness
// harnesses pick them up for free. That was wrong: TestGPUMatchesCPU renders
// its own equivScene and TestRenderScaleConsistency renders renderref.Scene, so
// neither ever saw StrokeHeavy, CurveHeavy, TextHeavy or UIScreen.
//
// It mattered immediately. Phase C2 moved 24 stroke outlines from
// stencil-then-cover to the convex tier — a different anti-aliasing method, on
// content no correctness test rendered. The parity number stayed identical
// through that change because the one scene under test contains a single path
// that moved.
//
// The tolerance matches TestGPUMatchesCPU: rasterizers differ at anti-aliased
// edges, so a small fraction of pixels may differ by a moderate amount while
// interiors, text coverage and clip boundaries must agree.
func TestGPUMatchesCPUOnCorpus(t *testing.T) {
	// Per-scene budgets recording what the backends actually agree on today,
	// so a regression fails rather than blending into one global average. They
	// are ceilings on current behaviour, not targets — the measured values are
	// in the comments, and the gap between a number and its budget is headroom
	// for edge-AA noise, not permission to drift.
	scenes := []struct {
		name    string
		root    widget.Widget
		maxFrac float64
	}{
		{"stroke-heavy", renderref.StrokeHeavy(), 0.02}, // measured 0.48%
		{"curve-heavy", renderref.CurveHeavy(), 0.02},   // measured 0.36%
		{"ui-screen", renderref.UIScreen(), 0.02},       // measured 0.62%
		{"mixed", renderref.Scene(), 0.03},              // measured 1.52%
		// text-heavy's 11.0% is diagnosed, and it is not anti-aliasing noise:
		// the two backends lay the same string out differently.
		// glyph_mask_engine.snapXGrid gives GPU glyphs positions accumulated
		// from *rounded* advances, while the CPU rasterizer and MeasureWidthIn
		// both use the shaper's unrounded ones. The error accumulates instead
		// of cancelling, so a 43-character line at 9px ends 9 pixels wider on
		// the GPU than on the CPU while starting on the same pixel.
		//
		// See TestGPUTextWidthVersusMeasuredWidth for the per-size numbers and
		// design/rendering-pipeline.md for why fixing it is a design decision
		// rather than a patch. Budgeted where it stands so it cannot grow.
		{"text-heavy", renderref.TextHeavy(), 0.12}, // measured 11.01%
	}
	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			h, err := NewHeadless(sc.root, Config{
				Size: renderref.SceneSize, Background: renderref.Background(),
				Font: goregular.TTF,
			}, 1)
			if err != nil {
				t.Fatal(err)
			}
			cpu := toRGBA(h.Render())
			gpuImg := h.RenderGPU()
			if gpuImg == nil {
				t.Skip("no headless GPU adapter")
			}
			gpu := toRGBA(gpuImg)
			if cpu.Bounds() != gpu.Bounds() {
				t.Fatalf("size mismatch: cpu %v vs gpu %v", cpu.Bounds(), gpu.Bounds())
			}

			const chanTol = 32
			var diffPixels, total, maxDiff int
			b := cpu.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					o := cpu.PixOffset(x, y)
					d := 0
					for k := range 4 {
						dd := int(cpu.Pix[o+k]) - int(gpu.Pix[o+k])
						if dd < 0 {
							dd = -dd
						}
						d = max(d, dd)
					}
					maxDiff = max(maxDiff, d)
					if d > chanTol {
						diffPixels++
					}
					total++
				}
			}
			frac := float64(diffPixels) / float64(total)
			t.Logf("%-13s differing pixels (>%d/chan): %d/%d = %.3f%%; max channel diff %d",
				sc.name, chanTol, diffPixels, total, frac*100, maxDiff)
			if frac > sc.maxFrac {
				t.Errorf("%s: GPU and CPU disagree on %.2f%% of pixels (budget %.2f%%)",
					sc.name, frac*100, sc.maxFrac*100)
			}
		})
	}
}
