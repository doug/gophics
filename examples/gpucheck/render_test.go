package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/doug/gophics/app"
	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
)

// TestGPUCheckRenders mounts the scene, advances a few animation frames, and
// (with GPUCHECK_SHOT set) writes the reference PNG.
func TestGPUCheckRenders(t *testing.T) {
	h, err := app.NewHeadless(gpucheck.Root(), gpucheck.Config(), 2)
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

// The scene draws its headings in the "bold" family. Config is what the
// mobile bind runs with, so the face has to be registered there: a Config
// with only Font fell through to the regular face on device, and the
// screenshot could not match the desktop reference.
func TestConfigRegistersTheBoldFace(t *testing.T) {
	cfg := gpucheck.Config()
	if len(cfg.FontFamilies["bold"]) == 0 {
		t.Fatal(`Config().FontFamilies has no "bold" face; the headings will draw regular`)
	}
	if cfg.Size.W == 0 || cfg.Size.H == 0 {
		t.Fatal("Config().Size is unset; the desktop window and the reference would differ")
	}
}
