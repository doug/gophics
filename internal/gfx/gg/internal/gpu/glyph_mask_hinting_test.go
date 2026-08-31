//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/text"
)

// TestGlyphMaskFullHintingForLatin guards the hinting choice: small axis-aligned
// Latin text uses FULL hinting so stems are grid-fit and crisp. layoutGlyphs
// then places fully-hinted glyphs on integer device pixels (so the grid-fit
// stems are not displaced and faded) using rounded advances (so spacing stays
// even). Skewed text disables hinting.
func TestGlyphMaskFullHintingForLatin(t *testing.T) {
	if h := selectGlyphMaskHinting(13, gg.Identity(), false, 1.0); h != text.HintingFull {
		t.Fatalf("small Latin text hinting = %v, want HintingFull", h)
	}
	if h := selectGlyphMaskHinting(13, gg.Matrix{A: 1, B: 0.3, D: 0.3, E: 1}, false, 1.0); h != text.HintingNone {
		t.Fatalf("skewed text hinting = %v, want HintingNone", h)
	}
}

// Letter-spacing evenness — that a word's internal advances do not depend on
// the sub-pixel position it starts at — used to be tested here by comparing
// GlyphMaskQuad.X0 values. That proxy stopped being valid when snapXGrid was
// removed: a quad's X0 is now an integer
// bucket and the rest of the position lives in which sub-pixel mask variant is
// drawn, so differences between X0 values read as ±1px jitter while nothing
// visible moves.
//
// The property is about what is drawn, so it is now asserted on what is drawn,
// in app.TestGPUTextSpacingIsIndependentOfSubpixelStart: the CPU rasterizer
// places glyphs at exact shaper positions, so GPU/CPU agreement across
// sub-pixel start offsets is the real test. Measured 1.0-1.9% at every offset
// with no outlier; a backend opening a gap would spike at the offsets where its
// rounding crossed a pixel boundary.
