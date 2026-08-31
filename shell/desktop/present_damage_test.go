//go:build !js

package desktop

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// The retained-texture upload must cover exactly the damaged rows.
//
// This is the arithmetic that decides whether a partial upload is correct:
// getting it wrong leaves pixels from a previous frame on screen, which no
// build error and no unit test of the renderer would catch. It is worth
// isolating because the rect arrives in physical pixels while everything that
// produced it worked in logical ones — the bug this path shipped with, where a
// 2x display uploaded the top half of the height and kept the rest.
func TestDamageRowsCoverTheDamagedRegion(t *testing.T) {
	const ph, stride = 1280, 1280 * 4
	for _, c := range []struct {
		name           string
		damage         geom.Rect
		wantLo, wantHi int
	}{
		{"middle band", geom.RectXYWH(0, 576, 1280, 46), 576, 622},
		{"clamped past bottom", geom.RectXYWH(0, 1200, 1280, 200), 1200, 1280},
		{"negative origin clamps to 0", geom.Rect{
			Min: geom.Pt{X: 0, Y: -40}, Max: geom.Pt{X: 1280, Y: 30}}, 0, 30},
		{"empty means nothing changed", geom.Rect{}, 0, 0},
		{"whole surface", geom.RectXYWH(0, 0, 1280, 1280), 0, 1280},
	} {
		t.Run(c.name, func(t *testing.T) {
			y0, y1 := damageRows(c.damage, ph)
			if y0 != c.wantLo || y1 != c.wantHi {
				t.Errorf("damageRows = %d..%d, want %d..%d", y0, y1, c.wantLo, c.wantHi)
			}
			if y1 > y0 {
				// The byte span handed to UpdateRegion must match the row count
				// exactly, or WriteTexture rejects it and the frame is dropped.
				if got, want := (y1-y0)*stride, len(make([]byte, (y1-y0)*stride)); got != want {
					t.Errorf("byte span %d, want %d", got, want)
				}
				if y1 > ph || y0 < 0 {
					t.Errorf("rows %d..%d escape the surface height %d", y0, y1, ph)
				}
			}
		})
	}
	// A rect that is entirely below the surface must upload nothing rather than
	// clamp to a backwards range.
	if y0, y1 := damageRows(geom.RectXYWH(0, 2000, 100, 50), ph); y1 > y0 {
		t.Errorf("a rect below the surface gave rows %d..%d, want an empty range", y0, y1)
	}
}
