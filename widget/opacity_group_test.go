package widget_test

import (
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A group at partial opacity must be partially visible.
//
// The gallery fades a card to 0.25 and back. Reported on a phone as fading to
// nothing at all, which is what a composite that drops or double-applies the
// group alpha looks like: 0.25 applied twice is 0.06, and indistinguishable
// from gone against a light background.
func TestOpacityGroupIsPartiallyVisible(t *testing.T) {
	const alpha = 0.25
	black := paint.RGB(0, 0, 0)

	render := func(a float32) (r, g, b uint32) {
		app_ := apptest.New(t, widget.Opacity{
			Alpha: a,
			Child: widget.Fill{Color: black, Child: widget.Sized{W: 40, H: 40}},
		}, apptest.WithConfig(app.Config{
			Size:       geom.Size{W: 40, H: 40},
			Background: paint.RGB(1, 1, 1), // white, so any ink darkens it
		}))
		img := app_.Render()
		c := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2)
		r, g, b, _ = c.RGBA()
		return
	}

	opaqueR, _, _ := render(1)
	fadedR, _, _ := render(alpha)
	clearR, _, _ := render(0)

	if opaqueR>>8 > 40 {
		t.Fatalf("fully opaque black group rendered light (r=%d); the test cannot "+
			"tell fading from not drawing", opaqueR>>8)
	}
	if clearR>>8 < 200 {
		t.Fatalf("alpha=0 still painted (r=%d)", clearR>>8)
	}

	// The whole point: 0.25 must land between the two, not on top of "gone".
	if fadedR >= clearR {
		t.Errorf("alpha=%v rendered as invisible as alpha=0 (r=%d vs %d) — "+
			"the group alpha is being dropped or applied twice", alpha, fadedR>>8, clearR>>8)
	}
	if fadedR <= opaqueR {
		t.Errorf("alpha=%v rendered as dark as fully opaque (r=%d vs %d) — "+
			"the group alpha is not being applied", alpha, fadedR>>8, opaqueR>>8)
	}
	t.Logf("opaque r=%d, alpha=%v r=%d, clear r=%d", opaqueR>>8, alpha, fadedR>>8, clearR>>8)
}
