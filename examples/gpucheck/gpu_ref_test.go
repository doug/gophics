//go:build gophics_gpu

package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/doug/gophics/app"
	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
)

// TestGPUReference renders the scene through the real GPU rasterizer (Metal on
// macOS) — the same ggcanvas/RenderDirect path the mobile surface uses — and
// writes the reference PNG the on-device screenshot is compared against.
// Run: GPUCHECK_GPU=<path> go test -tags gophics_gpu -run TestGPUReference ./examples/gpucheck
func TestGPUReference(t *testing.T) {
	out := os.Getenv("GPUCHECK_GPU")
	if out == "" {
		t.Skip("set GPUCHECK_GPU=<path>")
	}
	h, err := app.NewHeadless(gpucheck.Root(), gpucheck.Config(), 2)
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
