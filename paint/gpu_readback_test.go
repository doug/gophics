//go:build gophics_gpu

package paint

// Headless GPU readback harness. Drives gophics's real GPU present path — a
// scene recorded onto a GPU-backed gg.Context (via ggcanvas) composited to an
// off-screen render target — with no window, then reads the pixels back as an
// image. This is how GPU output is verified during development instead of
// eyeballing an on-screen window (which can't be captured in CI).
//
// Run: go test -tags gophics_gpu ./paint -run TestGPUReadback -v
// It writes PNGs to the paint package dir (gpu_*.png) for visual inspection and
// asserts basic invariants (text is not a solid block; background shows through).

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	_ "github.com/doug/gophics/internal/gfx/gg/gpu" // register gg's GPU accelerator
	"github.com/doug/gophics/internal/gfx/gg/integration/ggcanvas"
	"github.com/doug/gophics/internal/gfx/gogpu"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
)

// renderGPU records draw onto a GPU-backed gg.Context and reads the composited
// result back as an image, exercising the exact ggcanvas.Draw+Render path the
// desktop GPU present uses.
func renderGPU(t *testing.T, w, h int, draw func(cc *gg.Context)) *image.RGBA {
	t.Helper()
	r, err := gogpu.NewHeadlessRenderer()
	if err != nil {
		t.Skipf("no headless GPU: %v", err)
	}
	provider := r.GPUContextProvider()
	ggc, err := ggcanvas.New(provider, w, h)
	if err != nil {
		t.Fatalf("ggcanvas.New: %v", err)
	}
	img, err := r.RenderToImage(w, h, func(dc *gogpu.Context) {
		if err := ggc.Draw(draw); err != nil {
			t.Errorf("ggc.Draw: %v", err)
		}
		if err := ggc.Render(dc.RenderTarget()); err != nil {
			t.Errorf("ggc.Render: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("RenderToImage: %v", err)
	}
	return img
}

// writePNG writes img for visual inspection, but only when GOPHICS_GPU_DUMP is
// set — so `go test` runs don't litter the package dir with PNGs.
func writePNG(t *testing.T, name string, img image.Image) {
	t.Helper()
	if os.Getenv("GOPHICS_GPU_DUMP") == "" {
		return
	}
	f, err := os.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	t.Logf("wrote %s", name)
}

// countOpaque returns how many pixels in r are (near) fully opaque.
func countOpaqueRect(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.Pix[y*img.Stride+x*4+3] >= 250 {
				n++
			}
		}
	}
	return n
}

func TestGPUReadback(t *testing.T) {
	const w, h = 900, 300
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	p.BeginOffscreen(geom.Size{W: w, H: h}, 1) // set scale=1, dims

	white := RGB(1, 1, 1)
	black := RGB(0, 0, 0)
	blue := RGB(0.16, 0.35, 0.86)

	short := "Short"
	long := "The quick brown fox jumps over the lazy dog — a long run of text"

	img := renderGPU(t, w, h, func(cc *gg.Context) {
		canvas := p.GPUCanvas(cc)
		canvas.Clear(white)
		canvas.FillRect(geom.Rect{Min: geom.Pt{X: 20, Y: 20}, Max: geom.Pt{X: 120, Y: 80}}, blue)
		canvas.TextIn("", short, geom.Pt{X: 20, Y: 140}, 40, black)
		canvas.TextIn("", long, geom.Pt{X: 20, Y: 220}, 28, black)
	})
	writePNG(t, "gpu_readback.png", img)

	// Sanity: the blue rect should be opaque; the background white.
	if got := img.Pix[(40*img.Stride)+60*4+3]; got < 250 {
		t.Errorf("blue rect not opaque at (60,40): a=%d", got)
	}

	// The long-text band must NOT be a solid block: black glyphs cover only a
	// fraction of their bounding box over the white background, so most pixels
	// stay light. A near-solid dark band is the bug we're hunting. (Clear now
	// paints an opaque background, so this counts dark text pixels, not alpha.)
	band := image.Rect(20, 196, 20+700, 232)
	dark := countDarkRect(img, band)
	total := band.Dx() * band.Dy()
	frac := float64(dark) / float64(total)
	t.Logf("long-text band: %d/%d dark (%.1f%%)", dark, total, frac*100)
	if frac > 0.5 {
		t.Errorf("long text renders as a near-solid block: %.1f%% dark (want <50%%)", frac*100)
	}
}

// countDarkRect counts pixels in r whose luminance is low (text ink over the
// light background).
func countDarkRect(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := y*img.Stride + x*4
			if int(img.Pix[o])+int(img.Pix[o+1])+int(img.Pix[o+2]) < 384 { // avg < 128
				n++
			}
		}
	}
	return n
}
