//go:build gophics_gpu

package app

// GPU-vs-CPU equivalence and benchmarks (M5 exit criterion). Renders the same
// recorded scene through the CPU rasterizer and the GPU rasterizer (headless
// readback) and asserts they agree within tolerance, and measures raster time.
//
//	go test -tags gophics_gpu ./app -run TestGPUMatchesCPU -v
//	go test -tags gophics_gpu ./app -run x -bench BenchmarkRaster -benchmem

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// testAtlas is a 16×16 image with four solid 8×8 color quadrants.
func testAtlas() *image.RGBA {
	a := image.NewRGBA(image.Rect(0, 0, 16, 16))
	quad := [4]color.RGBA{
		{220, 60, 60, 255}, {60, 160, 220, 255}, {80, 190, 110, 255}, {230, 190, 60, 255},
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			a.Set(x, y, quad[(y/8)*2+(x/8)])
		}
	}
	return a
}

// equivScene draws the primitives the two backends must agree on: fills, rounded
// rects, a gradient, a stroke, a clipped fill, a filled path, and text at two sizes.
func equivScene() widget.Widget {
	white := paint.RGB(1, 1, 1)
	black := paint.RGB(0, 0, 0)
	return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(white)
		c.FillRect(geom.RectXYWH(20, 20, 100, 60), paint.RGB(0.85, 0.2, 0.2))
		c.FillRRect(geom.RectXYWH(140, 20, 120, 60), 16, paint.RGB(0.2, 0.4, 0.85))
		c.FillRRectGradient(geom.RectXYWH(20, 100, 240, 60), 12,
			paint.RGB(0.2, 0.85, 0.9), paint.RGB(0.9, 0.3, 0.55), true)
		c.StrokeRRect(geom.RectXYWH(20, 180, 240, 50), 10, 3, paint.RGB(0.1, 0.6, 0.2))
		c.PushClipRRect(geom.RectXYWH(20, 250, 240, 80), 20)
		c.FillRect(geom.RectXYWH(20, 250, 240, 80), paint.RGB(0.5, 0.3, 0.75))
		c.PopClip()
		tri := paint.NewPath()
		tri.MoveTo(geom.Pt{X: 150, Y: 330})
		tri.LineTo(geom.Pt{X: 250, Y: 330})
		tri.LineTo(geom.Pt{X: 200, Y: 252})
		c.FillPath(tri.Close(), paint.RGB(0.95, 0.55, 0.1))
		zig := paint.NewPath()
		zig.MoveTo(geom.Pt{X: 20, Y: 300}).LineTo(geom.Pt{X: 70, Y: 260}).
			LineTo(geom.Pt{X: 110, Y: 300}).LineTo(geom.Pt{X: 140, Y: 262})
		c.StrokePath(zig, 5, paint.RGB(0.1, 0.5, 0.7))
		atlas := testAtlas()
		c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(8, 0, 16, 8), Dst: geom.RectXYWH(150, 300, 16, 16)})
		c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(0, 8, 8, 16), Dst: geom.RectXYWH(170, 300, 16, 16), FlipX: true})
		c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(8, 8, 16, 16), Dst: geom.RectXYWH(190, 300, 16, 16), Tint: paint.RGB(0.5, 0.5, 1)})
		c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(0, 0, 8, 8), Dst: geom.RectXYWH(210, 295, 20, 20), Rotation: 0.5})
		c.TextIn("", "Equivalence", geom.Pt{X: 20, Y: 380}, 28, black)
		c.TextIn("", "gpu == cpu?", geom.Pt{X: 20, Y: 412}, 16, paint.RGB(0.4, 0.4, 0.45))
	}}
}

func equivHarness(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(equivScene(), Config{Size: geom.Size{W: 300, H: 440}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestGPUMatchesCPU(t *testing.T) {
	h := equivHarness(t)
	cpu := toRGBA(h.Render())
	gpuImg := h.RenderGPU()
	if gpuImg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu := toRGBA(gpuImg)

	if cpu.Bounds() != gpu.Bounds() {
		t.Fatalf("size mismatch: cpu %v vs gpu %v", cpu.Bounds(), gpu.Bounds())
	}
	if os.Getenv("GOPHICS_GPU_DUMP") != "" {
		dumpPNG(t, "/tmp/equiv_cpu.png", cpu)
		dumpPNG(t, "/tmp/equiv_gpu.png", gpu)
	}

	// Compare per pixel. Rasterizers differ slightly at anti-aliased edges, so
	// allow a small fraction of pixels to differ by a moderate amount; the bulk
	// (flat interiors, text coverage, clip boundaries) must match closely.
	const chanTol = 32 // per-channel abs difference tolerated per pixel
	var diffPixels, total int
	var maxDiff int
	b := cpu.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := cpu.PixOffset(x, y)
			d := 0
			for k := 0; k < 4; k++ {
				dd := int(cpu.Pix[o+k]) - int(gpu.Pix[o+k])
				if dd < 0 {
					dd = -dd
				}
				if dd > d {
					d = dd
				}
			}
			if d > maxDiff {
				maxDiff = d
			}
			if d > chanTol {
				diffPixels++
			}
			total++
		}
	}
	frac := float64(diffPixels) / float64(total)
	t.Logf("differing pixels (>%d/chan): %d/%d = %.3f%%; max channel diff %d",
		chanTol, diffPixels, total, frac*100, maxDiff)
	if frac > 0.05 {
		t.Errorf("GPU and CPU disagree on %.2f%% of pixels (want <5%%)", frac*100)
	}
}

// TestGPUGradientInterpolates verifies the GPU backend renders a linear
// gradient as an actual gradient (via clipped solid strips), not a flat fill:
// the left and right of a horizontal cyan→magenta band must differ, and a
// mid-band sample must sit between the two endpoints.
func TestGPUGradientInterpolates(t *testing.T) {
	from, to := paint.RGB(0.1, 0.9, 0.9), paint.RGB(0.9, 0.2, 0.5)
	scene := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(paint.RGB(1, 1, 1))
		c.FillRRectGradient(geom.RectXYWH(10, 10, 200, 60), 0, from, to, true)
	}}
	h, err := NewHeadless(scene, Config{Size: geom.Size{W: 220, H: 80}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	gimg := h.RenderGPU()
	if gimg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu := toRGBA(gimg)
	left := gpu.RGBAAt(20, 40)
	mid := gpu.RGBAAt(110, 40)
	right := gpu.RGBAAt(200, 40)
	d := absi(int(left.R)-int(right.R)) + absi(int(left.G)-int(right.G)) + absi(int(left.B)-int(right.B))
	t.Logf("gradient band L=%v M=%v R=%v endpoint diff=%d", left, mid, right, d)
	if d < 150 {
		t.Errorf("GPU gradient not interpolating: L↔R diff only %d", d)
	}
	// The midpoint red channel should lie strictly between the endpoints'
	// (cyan R≈26 → magenta R≈229), confirming a genuine ramp.
	if !(int(left.R) < int(mid.R) && int(mid.R) < int(right.R)) {
		t.Errorf("midpoint not between endpoints: L.R=%d M.R=%d R.R=%d", left.R, mid.R, right.R)
	}
}

// TestGPUOpacityGroup is the minimal repro for GPU opacity-layer compositing
// . A base fill, then a PushOpacity(0.5) group
// containing an overlapping fill. Before GPU layers, the accelerator lost the
// base content and ignored the group alpha; now the base must survive and the
// group must composite at ~50%. Checked both against the (correct) CPU path and
// via direct pixel assertions.
func TestGPUOpacityGroup(t *testing.T) {
	skipWithoutHardwareGPU(t)
	red := paint.RGB(1, 0, 0)
	blue := paint.RGB(0, 0, 1)
	// The overlay extends to the bottom-right corner (200,200): if a layer
	// target were sized in logical rather than physical pixels (the HiDPI
	// opacity bug class), the corner of the composite would be clipped at 2×/3×.
	scene := func() widget.Widget {
		return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
			c.Clear(paint.RGB(1, 1, 1))
			c.FillRect(geom.RectXYWH(0, 0, 120, 120), red) // base
			c.PushOpacity(0.5)
			c.FillRect(geom.RectXYWH(60, 60, 140, 140), blue) // half-opacity overlay to corner
			c.PopOpacity()
		}}
	}

	// Whole-frame GPU-vs-CPU agreement across device scales (the CPU path is the
	// correct reference). 2×/3× guard the physical-pixel target sizing.
	for _, scale := range []float32{1, 2, 3} {
		h, err := NewHeadless(scene(), Config{Size: geom.Size{W: 200, H: 200}}, scale)
		if err != nil {
			t.Fatal(err)
		}
		gimg := h.RenderGPU()
		if gimg == nil {
			t.Skip("no headless GPU adapter")
		}
		gpu := toRGBA(gimg)
		cpu := toRGBA(h.Render())
		if cpu.Bounds() != gpu.Bounds() {
			t.Fatalf("scale %v: size mismatch cpu %v gpu %v", scale, cpu.Bounds(), gpu.Bounds())
		}
		if os.Getenv("GOPHICS_GPU_DUMP") != "" {
			dumpPNG(t, "/tmp/opacity_gpu.png", gpu)
			dumpPNG(t, "/tmp/opacity_cpu.png", cpu)
		}

		var diff, total int
		b := gpu.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				o := gpu.PixOffset(x, y)
				d := 0
				for k := 0; k < 4; k++ {
					if dd := absi(int(cpu.Pix[o+k]) - int(gpu.Pix[o+k])); dd > d {
						d = dd
					}
				}
				if d > 32 {
					diff++
				}
				total++
			}
		}
		frac := float64(diff) / float64(total)
		t.Logf("scale %v: opacity group GPU vs CPU %d/%d = %.3f%% differ", scale, diff, total, frac*100)
		if frac > 0.07 { // AA tolerance rises with HiDPI (per the primitives gate)
			t.Errorf("scale %v: GPU opacity group disagrees with CPU on %.2f%% (want <7%%)", scale, frac*100)
		}

		// Direct property checks at 1× (the bug's symptoms), in physical px.
		if scale == 1 {
			const tol = 24
			near := func(name string, x, y int, want color.RGBA) {
				t.Helper()
				got := gpu.RGBAAt(x, y)
				if absi(int(got.R)-int(want.R)) > tol || absi(int(got.G)-int(want.G)) > tol ||
					absi(int(got.B)-int(want.B)) > tol {
					t.Errorf("%s @(%d,%d): got %v, want ~%v", name, x, y, got, want)
				}
			}
			near("base-survives", 30, 30, color.RGBA{255, 0, 0, 255})            // red survives the group
			near("overlay-over-white", 170, 170, color.RGBA{128, 128, 255, 255}) // 0.5*blue+0.5*white
			near("overlap-half", 90, 90, color.RGBA{128, 0, 128, 255})           // 0.5*blue+0.5*red (alpha applies)
		}
	}
}

// TestGPUOpacityNested exercises nested opacity groups (the resolveDraws
// recursion / child-of-child render path): an outer 0.5 group containing a fill
// and an inner 0.5 group, so the inner content lands at 0.25 effective opacity.
// The correct CPU path is the reference.
func TestGPUOpacityNested(t *testing.T) {
	skipWithoutHardwareGPU(t)
	scene := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(paint.RGB(1, 1, 1))
		c.FillRect(geom.RectXYWH(0, 0, 100, 100), paint.RGB(1, 0, 0))
		c.PushOpacity(0.5)
		c.FillRect(geom.RectXYWH(40, 40, 120, 120), paint.RGB(0, 1, 0)) // 0.5
		c.PushOpacity(0.5)
		c.FillRect(geom.RectXYWH(80, 80, 120, 120), paint.RGB(0, 0, 1)) // 0.25 effective
		c.PopOpacity()
		c.PopOpacity()
	}}
	h, err := NewHeadless(scene, Config{Size: geom.Size{W: 200, H: 200}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	gimg := h.RenderGPU()
	if gimg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu, cpu := toRGBA(gimg), toRGBA(h.Render())
	var diff, total int
	b := gpu.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := gpu.PixOffset(x, y)
			d := 0
			for k := 0; k < 4; k++ {
				if dd := absi(int(cpu.Pix[o+k]) - int(gpu.Pix[o+k])); dd > d {
					d = dd
				}
			}
			if d > 32 {
				diff++
			}
			total++
		}
	}
	frac := float64(diff) / float64(total)
	t.Logf("nested opacity GPU vs CPU: %d/%d = %.3f%% differ", diff, total, frac*100)
	if frac > 0.07 {
		t.Errorf("nested GPU opacity disagrees with CPU on %.2f%% (want <7%%)", frac*100)
	}
}

// TestGPUShadowThenFill reproduces the solitaire "black cards" bug: a drop
// shadow (semi-transparent black fills) drawn BEFORE an opaque white fill at the
// same rect. In insertion order the white face covers the shadow (a light card
// with a shadow halo). If the GPU reorders same-tier fills, the black shadow
// paints over the white face and the card centre goes dark. The CPU path is the
// reference.
func TestGPUShadowThenFill(t *testing.T) {
	felt := paint.RGB(0.10, 0.44, 0.30)
	white := paint.RGB(0.99, 0.99, 0.98)
	r := geom.RectXYWH(60, 40, 90, 120) // a "card"
	scene := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(felt)
		paint.DropShadow(c, r, 9, geom.Pt{Y: 2}, 5, paint.Color{R: 0, G: 0, B: 0, A: 0.28})
		c.FillRRect(r, 9, white) // the card face — must end up on top
	}}
	h, err := NewHeadless(scene, Config{Size: geom.Size{W: 220, H: 220}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	gimg := h.RenderGPU()
	if gimg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu, cpu := toRGBA(gimg), toRGBA(h.Render())

	// Sample the card centre — should be ~white on both paths.
	cx, cy := 105, 100
	gc, cc := gpu.RGBAAt(cx, cy), cpu.RGBAAt(cx, cy)
	t.Logf("card centre — cpu %v, gpu %v", cc, gc)
	if cc.R < 200 {
		t.Fatalf("CPU reference is wrong: centre %v not white", cc)
	}
	if gc.R < 200 || gc.G < 200 || gc.B < 200 {
		t.Errorf("GPU card centre is dark: got %v, want ~white (shadow painted over the face)", gc)
	}
}

func absi(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func toRGBA(img image.Image) *image.RGBA {
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

// BenchmarkRasterCPU measures a full-scene CPU rasterization (record once, then
// replay+raster each iteration).
func BenchmarkRasterCPU(b *testing.B) {
	h, err := NewHeadless(equivScene(), Config{Size: geom.Size{W: 300, H: 440}, Font: goregular.TTF}, 1)
	if err != nil {
		b.Fatal(err)
	}
	h.core.Layout(h.size)
	h.core.RecordScene(h.size, h.scale)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := h.core.Painter.BeginOffscreen(h.size, h.scale)
		h.core.ReplayScene(c)
		_ = h.core.Painter.Image()
	}
}

// BenchmarkRasterGPU measures the headless GPU path. NOTE: this includes a
// full texture→CPU readback each iteration, which on-screen presentation does
// not do — so it overstates GPU cost and is a loose upper bound, not the
// on-screen number.
func BenchmarkRasterGPU(b *testing.B) {
	h, err := NewHeadless(equivScene(), Config{Size: geom.Size{W: 300, H: 440}, Font: goregular.TTF}, 1)
	if err != nil {
		b.Fatal(err)
	}
	if h.RenderGPU() == nil {
		b.Skip("no headless GPU adapter")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.RenderGPU()
	}
}

func dumpPNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, img)
	t.Logf("wrote %s", path)
}
