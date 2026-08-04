package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/renderref"
)

// TestRenderScaleConsistency renders the reference scene at 1×, 2×, and 3× and
// requires the results to be structurally identical once resolution is
// normalized. Each render is reduced to a coarse cell grid (box-averaged, which
// cancels the anti-aliasing differences between scales) and compared cell by
// cell; a scale-dependent rendering bug — e.g. an opacity/clip/transform layer
// clipped on HiDPI — shows up as whole coarse cells that go missing at 2×/3×.
//
// This is the regression guard for the class of bug where a primitive is
// silently wrong only at deviceScale > 1 (the default on Retina/mobile), which
// per-pixel same-scale parity tests don't exercise.
func TestRenderScaleConsistency(t *testing.T) {
	const cw, ch = 64, 92 // coarse grid (~5 logical px/cell over 320×460)

	sig := func(scale float32) []float64 {
		h, err := NewHeadless(renderref.Scene(), Config{
			Size: renderref.SceneSize, Background: renderref.Background(),
			Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
		}, scale)
		if err != nil {
			t.Fatal(err)
		}
		return coarse(toRGBAImg(h.Render()), cw, ch)
	}

	ref := sig(1)
	for _, scale := range []float32{2, 3} {
		got := sig(scale)
		var bad, worst int
		var worstAt int
		for i := 0; i < len(ref); i += 3 {
			d := maxChan(ref[i:i+3], got[i:i+3])
			if d > worst {
				worst, worstAt = d, i/3
			}
			if d > coarseTol {
				bad++
			}
		}
		cells := cw * ch
		frac := float64(bad) / float64(cells)
		t.Logf("scale %g vs 1×: %d/%d coarse cells differ (>%d) = %.2f%%; worst diff %d at cell (%d,%d)",
			scale, bad, cells, coarseTol, frac*100, worst, worstAt%cw, worstAt/cw)
		if frac > coarseMaxFrac {
			t.Errorf("scale %g diverges from 1× on %.2f%% of coarse cells (want <%.1f%%) — a HiDPI-dependent render bug?",
				scale, frac*100, coarseMaxFrac*100)
		}
	}
}

const (
	coarseTol = 26 // per-channel avg-difference tolerated per coarse cell
	// Fraction of coarse cells allowed to exceed coarseTol. Calibrated: a
	// correct renderer differs ~6.7% across scales (unavoidable AA/text-hinting
	// slack at sharp edges); the HiDPI opacity-clipping bug this guards against
	// diverged ~27% (whole regions missing). 15% splits them with ~2× margin.
	coarseMaxFrac = 0.15
)

// coarse box-averages img into a cw×ch grid, returning cw*ch*3 channel means
// (0..255, RGB). Averaging over a cell cancels sub-pixel AA differences between
// scales while preserving whether a region has content.
func coarse(img *image.RGBA, cw, ch int) []float64 {
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	out := make([]float64, cw*ch*3)
	cnt := make([]int, cw*ch)
	for y := 0; y < H; y++ {
		cy := y * ch / H
		for x := 0; x < W; x++ {
			cx := x * cw / W
			o := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			ci := (cy*cw + cx) * 3
			out[ci] += float64(img.Pix[o])
			out[ci+1] += float64(img.Pix[o+1])
			out[ci+2] += float64(img.Pix[o+2])
			cnt[cy*cw+cx]++
		}
	}
	for i := 0; i < cw*ch; i++ {
		if cnt[i] > 0 {
			out[i*3] /= float64(cnt[i])
			out[i*3+1] /= float64(cnt[i])
			out[i*3+2] /= float64(cnt[i])
		}
	}
	return out
}

func maxChan(a, b []float64) int {
	m := 0.0
	for k := 0; k < 3; k++ {
		d := a[k] - b[k]
		if d < 0 {
			d = -d
		}
		if d > m {
			m = d
		}
	}
	return int(m)
}

func toRGBAImg(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	r := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r.Set(x, y, img.At(x, y))
		}
	}
	return r
}
