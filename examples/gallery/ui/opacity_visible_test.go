package ui

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

// The grouped-opacity demo has to be visible at the opacity it fades to, or it
// demonstrates nothing.
//
// It used to fade a Card, which in a light theme is a white surface on an
// off-white page: about six levels of contrast at full opacity, and under two
// once faded to a quarter. Reported from a phone as the card vanishing
// entirely — which it did, on every renderer. This measures the faded panel
// against the page so the demo cannot quietly regress to invisible again.
func TestFadedOpacityPanelStaysVisible(t *testing.T) {
	// Mounted the way the catalog mounts it: sectionPage supplies the scaffold
	// and the themed page background. Bare, the section renders on black and
	// any panel looks high-contrast, which would make this pass on nothing.
	a := apptest.New(t, sectionPage{
		title:    "Cards & surfaces",
		subtitle: "Surfaces, borders, and group opacity",
		body:     cardsSection{},
	}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 900}, Font: goregular.TTF,
	}))

	// Drive the section into its faded state through its own control.
	a.TapText("Grouped opacity")
	for i := 0; i < 60; i++ {
		a.Step(1.0 / 60)
	}

	// Measure inside the faded panel only. Measuring the whole page would find
	// the section heading and the Filled/Bordered boxes and pass no matter what
	// the panel did — which it did on the first attempt at this test.
	n := a.NodeContaining("Grouped opacity")
	if n == nil {
		t.Fatalf("faded panel not found; labels=%v", a.Labels())
	}

	img := a.Render()
	page := img.At(4, 4)
	pr, pg, pb, _ := page.RGBA()

	maxDist := 0
	x0, y0 := int(n.Rect.Min.X)+4, int(n.Rect.Min.Y)+4
	x1, y1 := int(n.Rect.Max.X)-4, int(n.Rect.Max.Y)-4
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			d := abs(int(r>>8)-int(pr>>8)) + abs(int(g>>8)-int(pg>>8)) + abs(int(b>>8)-int(pb>>8))
			if d > maxDist {
				maxDist = d
			}
		}
	}

	t.Logf("panel rect %.0fx%.0f, greatest contrast against the page = %d",
		n.Rect.Dx(), n.Rect.Dy(), maxDist)
	if maxDist < 90 {
		t.Errorf("the faded panel differs from the page by at most %d — it is "+
			"invisible at the alpha it fades to, so the demo shows nothing", maxDist)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
