//go:build gossamer_gpu

package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	gpucheck "github.com/doug/gossamer/examples/gpucheck/ui"
	"github.com/doug/gossamer/geom"
)

// TestGPUReference renders the scene through the real GPU rasterizer (Metal on
// macOS) — the same ggcanvas/RenderDirect path the mobile surface uses — and
// writes the reference PNG the on-device screenshot is compared against.
// Run: GPUCHECK_GPU=<path> go test -tags gossamer_gpu -run TestGPUReference ./examples/gpucheck
func TestGPUReference(t *testing.T) {
	out := os.Getenv("GPUCHECK_GPU")
	if out == "" {
		t.Skip("set GPUCHECK_GPU=<path>")
	}
	h, err := app.NewHeadless(gpucheck.Root(), app.Config{
		Size: geom.Size{W: 440, H: 660}, Background: gpucheck.Background(),
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		h.Step(0.016)
	}
	img := h.RenderGPU()
	if img == nil {
		t.Skip("no headless GPU adapter")
	}
	f, _ := os.Create(out)
	defer f.Close()
	_ = png.Encode(f, img)
}
