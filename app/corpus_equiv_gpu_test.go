//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/gfx/gg"
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
		// text-heavy has moved twice and the history is the point.
		//
		// It was 11.0% when the GPU placed glyphs on a grid of accumulated
		// rounded advances: that drifted a line up to 9px across 43 characters
		// away from the width MeasureWidthIn had promised, and the divergence
		// was a real bug. Removing the snap took it to 4.98% and replaced the
		// drift with a worse artefact — glyphs drawing from four different
		// sub-pixel masks, so weight varied inside a word.
		//
		// The GPU now rounds each glyph's own position to a whole device
		// pixel, which is bounded (half a pixel, never accumulating) and puts
		// every glyph on the fracX=0 mask. That is deliberately *not* what the
		// CPU rasterizer does — it draws unhinted at exact sub-pixel positions
		// — so the two backends now disagree by design about where a glyph
		// sits, not about how to rasterize one. Hence 7.35% rather than 4.98%:
		// a real difference in placement policy, not a defect.
		//
		// I first wrote that aligning the CPU path would close the gap and make
		// small text crisper there too. That was wrong, and the reason is worth
		// keeping: the GPU snaps *because* it grid-fits. Full hinting moves a
		// glyph's stems onto the pixel grid, and an outline fitted that way is
		// only crisp if it is then drawn on that grid. The CPU rasterizer draws
		// unhinted — HintingNone, everywhere, no exceptions — so snapping it
		// would quantize positions without any grid-fitting to justify it:
		// uniformity bought with the positional accuracy that sub-pixel
		// placement gives, and nothing gained in return.
		//
		// So each path is internally consistent — hinted snaps, unhinted does
		// not — and the divergence is the visible consequence of that, not a
		// defect waiting to be closed. Making the numbers agree would mean
		// changing one path's quality policy to match the other's, and neither
		// direction is an improvement on its own terms.
		{"text-heavy", renderref.TextHeavy(), 0.08}, // measured 7.35%
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
			dx0, dy0 := 1<<30, 1<<30
			dx1, dy1 := -1, -1
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
						dx0, dy0 = min(dx0, x), min(dy0, y)
						dx1, dy1 = max(dx1, x), max(dy1, y)
					}
					total++
				}
			}
			frac := float64(diffPixels) / float64(total)
			t.Logf("%-13s differing pixels (>%d/chan): %d/%d = %.3f%%; max channel diff %d",
				sc.name, chanTol, diffPixels, total, frac*100, maxDiff)
			// Where the disagreement lives, not just how much. A backend that
			// gets one region wrong and a backend that is uniformly noisy
			// produce the same percentage and want different investigations.
			if diffPixels > 0 {
				t.Logf("%-13s disagreement bounds: x %d..%d  y %d..%d",
					sc.name, dx0, dx1, dy0, dy1)
			}
			// Budgets gate a real GPU only. A software adapter takes
			// strategyRasterAtlas — shapes route to the CPU rasterizer and the
			// "GPU" composites — which is a different pipeline answering a
			// different question, so its numbers are reported and not gated.
			//
			// They are worth reading. On the UTM software renderer three
			// scenes beat every real GPU — curve-heavy is pixel-exact.
			//
			// `mixed` used to disagree on 18.8%, scene-wide, and is now 0.85%.
			// The cause is settled rather than guessed: across the two runs the
			// other four scenes are byte-identical, and mixed is the only corpus
			// scene containing opacity groups. Folding single-draw groups
			// removed all twenty of them — the software adapter had been
			// compositing those offscreen layers differently, and there are no
			// layers left to composite.
			if softwareAdapter() {
				t.Logf("%-13s (software adapter: reported, not gated)", sc.name)
				return
			}
			if frac > sc.maxFrac {
				t.Errorf("%s: GPU and CPU disagree on %.2f%% of pixels (budget %.2f%%)",
					sc.name, frac*100, sc.maxFrac*100)
			}
		})
	}
}

// softwareAdapter reports whether the accelerator is a CPU adapter.
func softwareAdapter() bool {
	a, ok := gg.Accelerator().(gg.AdapterAware)
	return ok && a.IsSoftwareAdapter()
}

// skipWithoutHardwareGPU skips a test that asserts a GPU *feature* works.
//
// The usual guard is a nil check on RenderGPU, which catches "no adapter" and
// lets a software adapter through — and a software adapter is not a small,
// slow GPU, it is a reduced feature set. Measured on the UTM Windows VM, whose
// "Software Renderer" backend renders shapes and text well (curve-heavy is
// pixel-exact against the CPU, better than either real GPU) while implementing
// neither backdrop blur nor opacity groups and routing nothing to the stencil
// tier.
//
// So these tests were failing on every machine without a GPU driver, which is a
// broken gate rather than a caught defect: they assert blur works, not that
// every adapter has blur. Parity tests do not use this — they report on a
// software adapter instead, because their numbers stay meaningful.
func skipWithoutHardwareGPU(t *testing.T) {
	t.Helper()
	if softwareAdapter() {
		t.Skip("software adapter: this GPU feature is not implemented there")
	}
}
