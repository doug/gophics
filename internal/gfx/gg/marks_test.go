package gg

import (
	"math"
	"testing"
)

// BlitMarks must agree with the path rasterizer. It bypasses it entirely, so
// nothing else would notice a drift in placement, coverage, or blending.
func TestBlitMarksMatchesPathFill(t *testing.T) {
	const w, h = 60, 60
	pts := [][2]float32{{15, 15}, {30, 31}, {45, 20}, {22, 44}}
	const d = 9.0

	blit := NewContext(w, h)
	blit.SetRGBA(1, 1, 1, 1)
	blit.Clear()
	xs := make([]float32, len(pts))
	ys := make([]float32, len(pts))
	cols := make([]RGBA, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = p[0], p[1]
		cols[i] = RGBA{R: 0.2, G: 0.4, B: 0.9, A: 1}
	}
	if !blit.BlitMarks(MarkCircle, d, xs, ys, cols) {
		t.Fatal("BlitMarks declined a plain axis-aligned context")
	}

	ref := NewContext(w, h)
	ref.SetRGBA(1, 1, 1, 1)
	ref.Clear()
	ref.SetRGBA(0.2, 0.4, 0.9, 1)
	for _, p := range pts {
		ref.DrawCircle(float64(p[0]), float64(p[1]), d/2)
		ref.Fill()
	}

	a, b := blit.Image(), ref.Image()
	var worst, sum int
	for y := range h {
		for x := range w {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			for _, d := range []int{int(ar>>8) - int(br>>8), int(ag>>8) - int(bg>>8), int(ab>>8) - int(bb>>8)} {
				if d < 0 {
					d = -d
				}
				sum += d
				worst = max(worst, d)
			}
		}
	}
	mean := float64(sum) / float64(w*h*3)
	// Antialiasing differs between a supersampled stamp and analytic coverage,
	// so edges are allowed to disagree; the shapes must still land together.
	if worst > 40 || mean > 0.6 {
		t.Errorf("blit vs path fill: worst channel diff %d, mean %.2f — the stamp "+
			"is not landing where the rasterizer puts the same circles", worst, mean)
	}
}

// A rotated transform has no axis-aligned stamp, so the batch must decline and
// let the caller fall back rather than draw circles where ellipses belong.
func TestBlitMarksDeclinesUnderRotation(t *testing.T) {
	c := NewContext(40, 40)
	c.Rotate(math.Pi / 6)
	if c.BlitMarks(MarkCircle, 8, []float32{20}, []float32{20}, []RGBA{{A: 1}}) {
		t.Error("BlitMarks accepted a rotated transform")
	}
}

// Marks outside the surface must be clipped, not written out of bounds.
func TestBlitMarksClipsToSurface(t *testing.T) {
	c := NewContext(20, 20)
	xs := []float32{-50, 70, 10, 10}
	ys := []float32{10, 10, -50, 70}
	cols := make([]RGBA, 4)
	for i := range cols {
		cols[i] = RGBA{R: 1, A: 1}
	}
	if !c.BlitMarks(MarkCircle, 6, xs, ys, cols) {
		t.Fatal("declined")
	} // must not panic
}

// A mismatched batch is a caller bug; drawing part of it would hide that.
func TestBlitMarksRejectsMismatchedBatch(t *testing.T) {
	c := NewContext(20, 20)
	if c.BlitMarks(MarkCircle, 6, []float32{1, 2}, []float32{1}, []RGBA{{A: 1}, {A: 1}}) {
		t.Error("accepted X and Y of different lengths")
	}
}
