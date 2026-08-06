//go:build gophics_gpu

package app

// GPU backdrop-blur guard: the blur must be confined to its region. Renders
// through the real GPU rasterizer (Metal on macOS, headless readback).
//
//	go test -tags gophics_gpu ./app -run TestBackdropBlurGPU

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// TestBackdropBlurGPUConfinedToRegion guards the GPU backdrop-blur scissor: a
// blur over the center of a hard black|white seam must blend the seam INSIDE the
// blurred box while leaving the seam OUTSIDE it a hard edge. An earlier bug let
// the frosted backdrop (a full-surface textured quad) cover the whole frame
// because the analytic rounded clip alone doesn't scissor a textured quad.
func TestBackdropBlurGPUConfinedToRegion(t *testing.T) {
	root := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.FillRect(geom.RectXYWH(0, 0, 100, 160), paint.RGB(0, 0, 0))   // left half black
		c.FillRect(geom.RectXYWH(100, 0, 100, 160), paint.RGB(1, 1, 1)) // right half white
		r := geom.RectXYWH(60, 60, 80, 40)                              // straddles the seam, mid-height
		c.PushClip(r)
		c.BackdropBlur(r, 10)
		c.PopClip()
	}}
	h, err := NewHeadless(root, Config{Size: geom.Size{W: 200, H: 160}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	img := h.RenderGPU()
	if img == nil {
		t.Skip("no headless GPU adapter")
	}
	red := func(x, y int) uint8 { r, _, _, _ := img.At(x, y).RGBA(); return uint8(r >> 8) }

	// Inside the blur, at the seam: a blend (mid-gray), not pure black or white.
	if r := red(100, 80); r < 40 || r > 215 {
		t.Errorf("seam inside the blur not blended: r=%d (want mid-gray)", r)
	}
	// Above the blur (y=20) the seam stays a hard edge — black left, white right —
	// proving the frost did not leak outside its scissor.
	if r := red(85, 20); r > 60 {
		t.Errorf("left of the seam above the blur is not black: r=%d", r)
	}
	if r := red(115, 20); r < 195 {
		t.Errorf("right of the seam above the blur is not white: r=%d", r)
	}
}
