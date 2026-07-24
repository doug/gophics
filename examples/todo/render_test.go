package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/doug/gossamer/geom"
)

// TestRenderOffscreen renders the app headless — no window, no GPU — and
// writes the result for inspection when GOSSAMER_RENDER_OUT is set.
// This is the seed of the golden-test harness (PLAN.md §8).
func TestRenderOffscreen(t *testing.T) {
	a := newApp()
	a.size = geom.Size{W: 440, H: 560}
	a.hover = 2 // exercise the hover + delete-button path
	a.input = "type here"

	c := a.painter.BeginOffscreen(a.size, 2)
	a.layout()
	a.draw(c)

	img := a.painter.Image()
	if img == nil {
		t.Fatal("no image produced")
	}
	b := img.Bounds()
	if b.Dx() != 880 || b.Dy() != 1120 {
		t.Fatalf("physical size = %dx%d, want 880x1120 (440x560 @2x)", b.Dx(), b.Dy())
	}

	if out := os.Getenv("GOSSAMER_RENDER_OUT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
