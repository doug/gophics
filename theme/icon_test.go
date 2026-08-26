package theme

import (
	"testing"

	"github.com/doug/gophics/paint"
)

// Every glyph draws something, inside its box.
//
// The point of shipping paths instead of recommending an icon font is that a
// path cannot come out empty. A glyph that silently drew nothing would be the
// same failure the Go fonts have — a blank where a control should be, and no
// error to say so — reintroduced by the thing meant to avoid it. A glyph that
// drew outside its box would be clipped, which is the same fault wearing a
// different hat.
func TestEveryGlyphDrawsInsideItsBox(t *testing.T) {
	names := map[IconGlyph]string{
		IconHome: "Home", IconList: "List", IconSearch: "Search", IconSliders: "Sliders",
		IconChevronLeft: "ChevronLeft", IconChevronRight: "ChevronRight",
		IconChevronUp: "ChevronUp", IconChevronDown: "ChevronDown",
		IconPlus: "Plus", IconClose: "Close", IconCheck: "Check", IconMenu: "Menu",
		IconCalendar: "Calendar", IconChart: "Chart",
	}
	// IconChart is the last constant; every value up to it must be covered, so
	// a glyph added without a shape is caught here rather than on screen.
	for g := IconHome; g <= IconChart; g++ {
		name, ok := names[g]
		if !ok {
			t.Errorf("glyph %d has no name in this test — was a constant added without a shape?", g)
			continue
		}
		const s = 1 // the 24x24 design grid, unscaled
		p := paint.NewPath()
		drawGlyph(p, g, s)
		if p.Empty() {
			t.Errorf("%s drew nothing", name)
			continue
		}
		b := p.Bounds()
		if b.Min.X < 0 || b.Min.Y < 0 || b.Max.X > 24 || b.Max.Y > 24 {
			t.Errorf("%s spans %v, outside its 24x24 box", name, b)
		}
		// And it fills a reasonable share of the box: a glyph collapsed into a
		// corner is technically non-empty and still looks broken.
		if b.Dx() < 6 && b.Dy() < 6 {
			t.Errorf("%s spans only %vx%v of its box", name, b.Dx(), b.Dy())
		}
	}
}
