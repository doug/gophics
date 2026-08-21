package gg

import (
	"math"
	"testing"
)

// A clip that starts off the left or top edge must clamp to zero, not wrap.
//
// setGPUClipRect converts the clip bounds to uint32 for the hardware scissor.
// A negative float converted to uint32 wraps to about 4.29e9, which makes the
// far edge compare as less than the near edge; the clip is then dropped for
// that draw and the drawing spills over whatever is beside it, instead of
// being trimmed. An off-screen edge is ordinary -- a card scrolled part-way
// off, or a transient mid-resize -- so it has to mean "start at zero".
func TestNegativeClipBoundsClampRatherThanWrap(t *testing.T) {
	// The unguarded conversion, kept here so the hazard is visible.
	if got := uint32(math.Floor(-1)); got != math.MaxUint32 {
		t.Fatalf("uint32(floor(-1)) = %d; this test assumes Go's wrapping conversion", got)
	}

	for _, tc := range []struct{ x, w float64 }{
		{-1, 100},  // one pixel off the left
		{-40, 200}, // well off the left
		{-0.5, 50}, // sub-pixel
	} {
		fx0 := math.Max(0, math.Floor(tc.x))
		fx1 := math.Ceil(tc.x + tc.w)
		if fx1 <= fx0 {
			t.Errorf("bounds x=%v w=%v collapsed after clamping (fx0=%v fx1=%v); "+
				"the clip would be dropped and the draw would spill", tc.x, tc.w, fx0, fx1)
			continue
		}
		if x0 := uint32(fx0); x0 != 0 {
			t.Errorf("bounds x=%v clamped to %d, want 0", tc.x, x0)
		}
	}
}
