package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// Home centers a 60×60 green hero; Detail puts a 120×120 green hero at the
// top-left over a red page. Pushing flies the hero from center to top-left.
type hHome struct{}

func (hHome) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { nav.Push(hDetail{}) }},
		Child: widget.Center(widget.Hero{Tag: "h",
			Child: widget.Decorated{Color: paint.RGB(0.3, 0.85, 0.4), Child: widget.Sized{W: 60, H: 60}}}),
	}
}

type hDetail struct{}

func (hDetail) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { nav.Pop() }},
		Child: widget.Stack{Children: []widget.Widget{
			widget.Fill{Color: paint.RGB(0.85, 0.2, 0.2)},
			widget.Align{X: 0, Y: 0, Child: widget.Hero{Tag: "h",
				Child: widget.Decorated{Color: paint.RGB(0.3, 0.85, 0.4), Child: widget.Sized{W: 120, H: 120}}}},
		}},
	}
}

// isGreen reports whether the pixel at logical (lx,ly) is the hero green
// (scale 2 → physical pixels).
func isGreen(img image.Image, lx, ly int) bool {
	r, g, b, _ := img.At(lx*2, ly*2).RGBA()
	return g > 0xA000 && r < 0x8000 && b < 0x8000
}

func heroHarness(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(widget.Navigator{Home: hHome{}},
		Config{Size: geom.Size{W: 300, H: 300}, Background: paint.RGB(0.12, 0.13, 0.16), Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func TestHeroFliesBetweenRoutes(t *testing.T) {
	h := heroHarness(t)

	// At rest, home's hero is centered: green at (150,150), not at the corner.
	if !isGreen(h.Render(), 150, 150) {
		t.Fatal("home hero should be centered at rest")
	}
	if isGreen(h.Render(), 20, 20) {
		t.Fatal("home hero should not cover the top-left corner")
	}

	h.Tap(geom.Pt{X: 150, Y: 150}) // push detail
	// Advance into the flight (rects lag one frame, so step a few).
	for i := 0; i < 5; i++ {
		h.Step(0.016)
		h.Render()
	}
	mid := h.Render()
	// Mid-flight the hero has left the center and is en route — present at an
	// intermediate point but not yet filling the destination's corner.
	if !isGreen(mid, 90, 90) {
		t.Fatal("hero should be mid-flight near the interpolated position")
	}
	if isGreen(mid, 20, 20) {
		t.Fatal("hero should not yet have reached the destination corner")
	}

	settle(h)
	end := h.Render()
	// Landed: detail's 120×120 hero fills the top-left; center is now red.
	if !isGreen(end, 20, 20) {
		t.Fatal("hero should land at the detail top-left after settling")
	}
	if isGreen(end, 200, 200) {
		t.Fatal("detail background (not hero) expected at (200,200)")
	}
}

func TestHeroReturnsOnPop(t *testing.T) {
	h := heroHarness(t)
	h.Tap(geom.Pt{X: 150, Y: 150}) // push
	settle(h)
	if !isGreen(h.Render(), 20, 20) {
		t.Fatal("precondition: hero at detail top-left")
	}

	h.Tap(geom.Pt{X: 60, Y: 60}) // tap the detail hero → pop
	settle(h)
	end := h.Render()
	// Back home: hero centered again, corner clear.
	if !isGreen(end, 150, 150) {
		t.Fatal("hero should return to the home center after pop")
	}
	if isGreen(end, 20, 20) {
		t.Fatal("home hero should not cover the corner after pop")
	}
}
