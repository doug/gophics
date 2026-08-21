package paint

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A clip pushed before a transform must still bound what the transform draws.
//
// solitaire's card back does exactly this: clip to the card, rotate 45 degrees,
// then tile squares well beyond the card and rely on the clip to trim them. If
// the clip were applied in the rotated space it would turn with the pattern and
// let it escape onto whatever is next to the card.
func TestClipBoundsRotatedDrawing(t *testing.T) {
	const W, H = 200, 200
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: W, H: H}, 1)
	c.Clear(RGB(0, 0, 0))

	card := geom.RectXYWH(50, 50, 100, 100)
	c.PushClipRRect(card, 8)
	c.PushTransform(Transform{Rotation: 0.7853982, PivotX: 100, PivotY: 100})
	c.FillRect(geom.RectXYWH(-300, -300, 800, 800), RGB(1, 0, 0)) // way outside
	c.PopTransform()
	c.PopClip()

	img := p.Image()
	outside := 0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			// generous margin for the rounded corners and antialiasing
			inCard := x >= 48 && x < 152 && y >= 48 && y < 152
			if r > 0x8000 && !inCard {
				outside++
			}
		}
	}
	if outside > 0 {
		t.Errorf("%d painted pixels escaped the clip — a rotated fill is not bounded "+
			"by a clip pushed before the transform, so a card's pattern can bleed "+
			"onto its neighbours", outside)
	}
}
