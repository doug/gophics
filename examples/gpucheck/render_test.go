package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
	"github.com/doug/gophics/geom"
)

// TestGPUCheckRenders mounts the scene, advances a few animation frames, and
// (with GPUCHECK_SHOT set) writes the reference PNG.
func TestGPUCheckRenders(t *testing.T) {
	h, err := app.NewHeadless(gpucheck.Root(), app.Config{
		Size: geom.Size{W: 440, H: 660}, Background: gpucheck.Background(),
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for range 30 {
		h.Step(0.016)
	}
	if h.Render().Bounds().Empty() {
		t.Fatal("empty render")
	}
	if out := os.Getenv("GPUCHECK_SHOT"); out != "" {
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, h.Render())
	}
}
