package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// headerH is the band above the navigator — the same shape a phone's
// safe-area inset produces when a notch pushes content down.
const headerH = 64

type hHeaderApp struct{}

func (hHeaderApp) Build(widget.Ctx) widget.Widget {
	col := widget.Column(
		widget.Decorated{Color: paint.RGB(0.1, 0.1, 0.1), Child: widget.Sized{H: headerH}},
		widget.Expand(widget.Navigator{Home: hHome{}}),
	)
	col.CrossAlign = layout.CrossStretch
	return col
}

// A hero lands where it belongs when the navigator does not start at the
// window origin.
//
// Hero rects are captured during paint, in window coordinates; the flight
// overlay is aligned to the navigator's own stack. Those agree only when the
// navigator is the whole window — which every other hero test here happens to
// arrange, so none of them can see this. Put a header above it, or run it on a
// phone where the safe-area inset does the same, and the flight is displaced
// by exactly that offset: it does not animate to the place it is going.
// greenBox is the bounding box of hero-green pixels, in logical coordinates.
func greenBox(img image.Image) (minX, minY, maxX, maxY int) {
	minX, minY, maxX, maxY = 9999, 9999, -1, -1
	for y := 0; y < 364; y += 2 {
		for x := 0; x < 300; x += 2 {
			if !isGreen(img, x, y) {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	return minX, minY, maxX, maxY
}

func TestAHeroLandsUnderAHeader(t *testing.T) {
	h, err := NewHeadless(hHeaderApp{},
		Config{Size: geom.Size{W: 300, H: 364}, Background: paint.RGB(0.12, 0.13, 0.16), Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// The navigator occupies y 64..364, so home's centred 60pt hero sits at
	// y 184..244 and detail's 120pt hero will sit at the top-left, y 64..184.
	// A flight between them therefore never leaves y 64..244.
	const navMidY = headerH + 150
	if !isGreen(h.Render(), 150, navMidY) {
		t.Fatal("home hero should be centred within the navigator, below the header")
	}

	h.Tap(geom.Pt{X: 150, Y: navMidY})
	for range 5 { // into the flight; rects lag a frame
		h.Step(0.016)
		h.Render()
	}

	_, top, _, bot := greenBox(h.Render())
	if top < 0 {
		t.Fatal("no hero visible mid-flight")
	}
	// The flight has to stay within the span of its own endpoints. Placed in
	// window coordinates inside a stack that already starts below the header,
	// it is pushed down by exactly the header height and runs past the bottom
	// endpoint — measured at y 160..262 against the correct 96..198.
	if bot > headerH+180 {
		t.Errorf("mid-flight the hero reaches y=%d, past the %d its endpoints span: "+
			"the flight was not converted out of window coordinates into the stack's",
			bot, headerH+180)
	}
	if top < headerH {
		t.Errorf("mid-flight the hero reaches y=%d, above the navigator's top edge %d", top, headerH)
	}

	settle(h)
	if !isGreen(h.Render(), 20, headerH+6) {
		t.Error("the hero did not land at the navigator's top-left corner")
	}
}
