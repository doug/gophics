//go:build gophics_gpu

package app

import (
	"math"
	"testing"

	"github.com/doug/gophics/chart"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"golang.org/x/image/font/gofont/goregular"
)

// A scatter must not cost a draw call per point on the GPU.
//
// paint.Marks exists because the CPU rasterizer charged per mark for deciding
// coverage it had already decided. That fast path is CPU-only — BlitMarks
// declines while an accelerator is active — which looks like the GPU being left
// out until you count: the tier system already coalesces like shapes, so ten
// thousand points arrive as a couple of dozen draws in one pass, and the cost
// that remains is fill rather than dispatch.
//
// Measured here so that stays true. A regression that split marks into
// per-point draws would not fail any correctness test, and on a desktop GPU it
// might not even be obvious — it would just quietly cost a phone its frame.
func TestGPUScatterStaysBatched(t *testing.T) {
	const n = 10_000
	data := make([]chart.Datum, n)
	for i := range data {
		f := float64(i)
		data[i] = chart.Datum{X: f, Y: math.Sin(f/97) * 50}
	}
	h, err := NewHeadless(
		chart.Chart{Marks: []chart.Mark{chart.PointMark{Data: data, Size: 3}}},
		Config{Size: geom.Size{W: 800, H: 500}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Warm up: the first frames compile pipelines and allocate attachments.
	for range 5 {
		if h.RenderGPU() == nil {
			t.Skip("no GPU adapter available")
		}
		h.Step(0.05)
	}

	h.Step(0.016)
	wgpu.ResetEncoderStats()
	h.RenderGPU()
	s := wgpu.EncoderStats()
	t.Logf("%d points → %d draw calls in %d pass(es)", n, s.DrawCalls, s.RenderPasses)

	// Generous: the chart draws axes, gridlines and labels besides the points,
	// and the budget is about the *shape* of the cost, not an exact figure.
	// Per-point dispatch would be three orders of magnitude past this.
	if s.DrawCalls > 100 {
		t.Errorf("%d draw calls for %d points — marks are no longer batched", s.DrawCalls, n)
	}
}
