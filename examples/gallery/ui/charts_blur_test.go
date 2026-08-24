package ui_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
)

// The charts page frosts nothing.
//
// A backdrop blur re-reads everything behind it, so its price is the content
// underneath drawn again — once per blur. Five frosted cards over five charts
// was the whole of the glass slowdown: 16.5ms median and 43ms worst frames,
// against 3.9 and 5.7 with the frosting gone. Neither glass nor charts is
// slow on its own, which is why both got blamed.
//
// This asserts the count rather than the time, because the time is a property
// of the machine and the count is a property of the page.
func TestTheChartsPageRecordsNoBackdropBlur(t *testing.T) {
	blurs := func(section string) int {
		a := apptest.New(t, ui.Gallery{}, apptest.WithConfig(app.Config{
			Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
		}))
		settle := func() {
			for s := 0; s < 40; s++ {
				a.Step(1.0 / 60)
			}
		}
		a.TapText("Glass")
		settle()
		a.Move(geom.Pt{X: 210, Y: 400})
		for i := 0; i < 20 && a.NodeContaining(section).Rect.Min.Y > 640; i++ {
			a.Scroll(geom.Pt{Y: -400})
			settle()
		}
		a.TapText(section)
		settle()
		a.Render()
		return a.Scene().BackdropBlurs()
	}

	// A page whose cards are still frosted, so the assertion below cannot pass
	// merely because the theme failed to take or nothing blurs anywhere.
	if n := blurs("Cards & surfaces"); n == 0 {
		t.Fatal("no glass page frosts anything; the control is broken, not the charts")
	}
	if n := blurs("Charts"); n != 0 {
		t.Errorf("the charts page records %d backdrop blurs; each costs the charts behind it drawn again", n)
	}
}
