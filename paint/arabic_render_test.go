package paint

import (
	"image/png"
	"os"
	"runtime"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
)

// TestShapedPipelineRender renders mixed-direction text through the full
// gossamer pipeline (shape → outlines → gg fill) when GOSSAMER_RENDER_OUT
// is set; it always asserts the fallback+shaping path produces glyphs.
func TestShapedPipelineRender(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("uses a macOS system font for Arabic")
	}
	arData, err := os.ReadFile("/System/Library/Fonts/SFArabic.ttf")
	if err != nil {
		t.Skip("SFArabic.ttf not present")
	}
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if err := p.LoadFallbackFont(arData); err != nil {
		t.Fatal(err)
	}

	const mixed = "peace: السلام عليكم :end"
	if l := p.Shape(mixed, 22); len(l.Glyphs) < 10 {
		t.Fatalf("mixed line glyphs = %d", len(l.Glyphs))
	}

	if out := os.Getenv("GOSSAMER_RENDER_OUT"); out != "" {
		c := p.BeginOffscreen(geom.Size{W: 480, H: 120}, 2)
		c.Clear(RGB(1, 1, 1))
		c.Text("السلام عليكم", geom.Pt{X: 20, Y: 50}, 30, RGB(0, 0, 0))
		c.Text(mixed, geom.Pt{X: 20, Y: 95}, 20, RGB(0.2, 0.2, 0.2))
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, p.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
