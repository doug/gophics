//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/internal/renderref"
	"github.com/doug/gophics/widget"
)

// Which tier draws a real UI, and does the encoder work scale with it?
//
// The tier model claims tier 2b — stencil-then-cover, the
// one tier with no anti-aliasing of its own — catches every stroke and every
// curved fill, and so most of a UI. Everything downstream depends on that: the
// 4× MSAA that exists to anti-alias 2b is what makes LoadOpLoad illegal, which
// is what corrupts multi-pass frames and refuses damage-rect scissoring. §4.6
// says that if 2b turns out to be a small minority, §3 is wrong and Phases C
// and D should be re-scoped before a line is written.
//
// So this reports rather than asserts a threshold; the numbers are the output.
// It fails only if the stencil tier draws nothing at all across the whole
// corpus, which would mean the plan is aimed at a tier nothing reaches.
func TestTierPopulationsAndEncoderWork(t *testing.T) {
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

	var totalStencil uint64
	for _, sc := range scenes {
		h, err := NewHeadless(sc.root, Config{Size: renderref.SceneSize, Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		if h.RenderGPU() == nil {
			t.Skip("no GPU adapter available")
		}
		skipWithoutHardwareGPU(t)
		h.RenderGPU() // a warm frame: the counters describe the last frame only

		tiers := gg.ReadFrameCounters()
		enc := wgpu.EncoderStats()
		totalStencil += tiers.Stencil

		t.Logf("%-13s tiers: sdf %3d  convex %3d  STENCIL %3d  image %2d  text %2d  glyph %2d  (2b = %d%% of %d)",
			sc.name, tiers.SDF, tiers.Convex, tiers.Stencil, tiers.Image,
			tiers.Text, tiers.GlyphMask, pct(tiers.Stencil, tiers.Total()), tiers.Total())
		t.Logf("%-13s encoder: %d passes, %d draws, %d pipeline switches  (F2 predicts switches ~= 2x stencil = %d)",
			sc.name, enc.RenderPasses, enc.DrawCalls, enc.PipelineSwitches, 2*tiers.Stencil)
		if tiers.DamageRefused > 0 {
			t.Logf("%-13s damage refused on %d frame(s) — F3b", sc.name, tiers.DamageRefused)
		}
	}

	if totalStencil == 0 {
		t.Error("the stencil tier drew nothing across the entire corpus: the tier " +
			"model is wrong, and the optimisation work aimed at tier 2b is aimed " +
			"at a tier nothing reaches")
	}
}

func pct(n, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	return n * 100 / total
}
