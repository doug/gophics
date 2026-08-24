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

// Mid-transition frames, rendered through the real GPU pipeline.
//
// Everything that tests a transition here today asserts on the semantic tree,
// which knows where a widget thinks it is and nothing about where it was
// drawn. An offset hero, a chart painting over a header, and a reveal that is
// not clipped are all faults in the pixels — invisible to a rect assertion,
// and reported by a person looking at a phone rather than by anything in this
// repository.
//
// Set TRANSITION_OUT to a directory to write every frame.
func TestTransitionFrames(t *testing.T) {
	out := os.Getenv("TRANSITION_OUT")
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

	target := "Charts"
	if !a.HasText(target) {
		t.Skipf("no %q section in the catalog", target)
	}

	// Frame 0: at rest.
	save("push_00_rest", a.RenderGPU())

	// Tap, then walk the push a few frames at a time, capturing each step.
	a.TapText(target)
	for i := 1; i <= 8; i++ {
		for s := 0; s < 3; s++ { // ~50ms per capture
			a.Step(1.0 / 60)
		}
		save(fmt.Sprintf("push_%02d_mid", i), a.RenderGPU())
	}

	// Settled.
	for s := 0; s < 40; s++ {
		a.Step(1.0 / 60)
	}
	settled := a.RenderGPU()
	save("push_99_settled", settled)

	// A touch fling, not a wheel scroll: a phone drags and releases, and the
	// momentum afterwards is when the chart was reported to paint over the
	// header. Wheel scrolling has no fling and may not exercise the same path.
	a.Drag(geom.Pt{X: 210, Y: 600}, geom.Pt{X: 210, Y: 150})
	for s := 0; s < 12; s++ {
		a.Step(1.0 / 60)
		save(fmt.Sprintf("fling_%02d", s), a.RenderGPU())
	}
	a.Drag(geom.Pt{X: 210, Y: 150}, geom.Pt{X: 210, Y: 700})
	for s := 0; s < 14; s++ {
		a.Step(1.0 / 60)
		save(fmt.Sprintf("flingback_%02d", s), a.RenderGPU())
	}

	a.Scroll(geom.Pt{Y: -400})
	for s := 0; s < 20; s++ {
		a.Step(1.0 / 60)
	}
	save("scroll_10_down", a.RenderGPU())
	a.Scroll(geom.Pt{Y: 400})
	for i := 1; i <= 6; i++ {
		for s := 0; s < 3; s++ {
			a.Step(1.0 / 60)
		}
		save(fmt.Sprintf("scroll_%02d_up", 10+i), a.RenderGPU())
	}
	t.Log("frames written; inspect them")
}
