package layoutbox

import (
	"github.com/doug/gophics/layout"
	"testing"

	"github.com/doug/gophics/geom"
)

// FracX/FracY shift a box by a fraction of its own size, which is the only way
// to place something relative to its own extent when that extent is not known
// until layout has run. A drag preview centred under a finger needs exactly
// that: the pointer position places the box, and half its own size pulls it
// back onto the point.
func TestTranslatedShiftsByAFractionOfItsOwnSize(t *testing.T) {
	for _, tc := range []struct {
		name         string
		fracX, fracY float32
		want         geom.Pt
	}{
		{"centred on the point", -0.5, -0.5, geom.Pt{X: -40, Y: -20}},
		{"unshifted", 0, 0, geom.Pt{X: 0, Y: 0}},
		{"a full size clear of it", -1, -1, geom.Pt{X: -80, Y: -40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &Translated{FracX: tc.fracX, FracY: tc.fracY, Child: &Sized{W: 80, H: 40}}
			if got := b.Layout(layout.Loose(geom.Size{W: 200, H: 200})); got != (geom.Size{W: 80, H: 40}) {
				t.Fatalf("laid out %v, want the child's 80x40", got)
			}
			if got := b.offsetPt(); got != tc.want {
				t.Errorf("offset %v, want %v", got, tc.want)
			}
		})
	}
}
