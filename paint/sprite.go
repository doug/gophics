package paint

import (
	"image"

	"github.com/doug/gossamer/geom"
)

// Sprite describes one blit of a source region of an atlas image into a
// destination rectangle. Passing the same atlas image.Image across many
// DrawSprite calls shares a single cached texture — the reason to use an atlas
// rather than one image per tile (which hits the texture-cache budget).
type Sprite struct {
	Src     image.Rectangle // source region in the atlas (pixels)
	Dst     geom.Rect       // destination (logical pixels)
	Alpha   float32         // 0 → 1 (opaque)
	FlipX   bool            // mirror horizontally about Dst's center
	Nearest bool            // nearest-neighbor sampling (crisp pixel art)
}
