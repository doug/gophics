//go:build gophics_gpu

package ui_test

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
)

// Frames of a Hero flight, rendered through the real GPU pipeline.
//
// A hero was reported as offset while it animates. The flight is an overlay
// copy interpolating between two rects recovered from the previous frame's
// paint, each with its page's transition slide undone (navigator.go,
// buildFlights/restRect) — so an offset is arithmetic on rects, and shows up
// in pixels rather than in the semantic tree, which never sees the overlay.
//
// The gallery's demo flies a 60x60 list swatch to a full-bleed 200-tall
// header, which is a large enough travel that a wrong rect is visible.
//
// Set HERO_OUT to a directory to write every frame.
func TestHeroFlightFrames(t *testing.T) {
	out := os.Getenv("HERO_OUT")
	a := apptest.New(t, ui.Gallery{}, apptest.Scale(2), apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
	}))
	if a.RenderGPU() == nil {
		t.Skip("no GPU adapter available")
	}

	save := func(name string, img image.Image) {
		if out == "" {
			return
		}
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	settle := func() {
		for s := 0; s < 40; s++ {
			a.Step(1.0 / 60)
		}
	}

	const section = "Navigator & Hero"
	if !a.HasText(section) {
		t.Skipf("no %q section in the catalog", section)
	}
	// The row sits well down the catalog. A tap is dispatched at the node's
	// centre whether or not that point is on screen, so scroll it into view
	// first — otherwise the tap lands outside the viewport and nothing opens.
	// Scroll goes to the last pointer position, so the pointer has to be over
	// the list before the wheel deltas mean anything.
	a.Move(geom.Pt{X: 210, Y: 400})
	for i := 0; i < 20 && a.NodeContaining(section).Rect.Min.Y > 640; i++ {
		a.Scroll(geom.Pt{Y: -400})
		settle()
	}
	if y := a.NodeContaining(section).Rect.Min.Y; y > 640 {
		t.Fatalf("could not scroll %q into view; it sits at y=%v", section, y)
	}
	catalog := a.Labels()
	a.TapText(section)
	settle()
	if sameLabels(catalog, a.Labels()) {
		t.Fatal("tapping the catalog row did not open the section")
	}
	save("hero_00_list", a.RenderGPU())

	// The card rows carry the hero swatches. Tapping a row pushes the detail
	// page whose header is the same tag, which is the flight under test.
	before := a.Labels()
	a.TapText("likes")
	for i := 1; i <= 10; i++ {
		for s := 0; s < 2; s++ { // ~33ms per capture, the flight is short
			a.Step(1.0 / 60)
		}
		save(fmt.Sprintf("hero_%02d_mid", i), a.RenderGPU())
	}
	settle()
	save("hero_99_settled", a.RenderGPU())

	if sameLabels(before, a.Labels()) {
		t.Fatal("tapping a card did not change the page; the flight never ran")
	}
}

func sameLabels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
