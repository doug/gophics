//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/text"
)

// A hinted glyph must land on a whole device pixel horizontally.
//
// X varies per glyph, so without snapping neighbouring glyphs in a word draw
// from different sub-pixel masks and the weight changes visibly across it.
func TestHintedGlyphsSnapXToWholeDevicePixels(t *testing.T) {
	for _, deviceScale := range []float64{1, 2, 3} {
		for _, absX := range []float64{0, 0.1, 0.33, 0.5, 0.67, 12.49, 12.51, 103.3} {
			gotX, _, fracX, _ := glyphPlacement(absX, 40.2, deviceScale, text.HintingFull)
			if fracX != 0 {
				t.Errorf("scale %.0f x=%.2f: fracX=%v, want 0", deviceScale, absX, fracX)
			}
			if dev := gotX * deviceScale; math.Abs(dev-math.Round(dev)) > 1e-9 {
				t.Errorf("scale %.0f: device x = %v, not a whole pixel", deviceScale, dev)
			}
		}
	}
}

// Y must NOT snap, and this is the difference between smooth scrolling and a
// stutter.
//
// A run shares one baseline, so every glyph in it has the same Y fraction and
// there is no unevenness for snapping to remove — it bought only grid-fit
// horizontal stems. What it cost was motion: a list creeping past at a third of
// a device pixel per frame held its text still for three frames and then jumped
// a whole pixel, so the text stepped while the rows behind it slid smoothly.
// That is the jitter at the tail of a flick, and why it looked fine while
// dragging, where a frame moves several pixels and hides it.
func TestHintedGlyphsMoveSmoothlyInY(t *testing.T) {
	const scale = 2.0
	var lastDev float64 = -1
	stalled := 0
	for i := range 24 {
		y := 40 + float64(i)*0.125 // an eighth of a logical pixel per frame
		_, absY, _, _ := glyphPlacement(10, y, scale, text.HintingFull)
		dev := absY * scale
		if lastDev >= 0 && dev == lastDev {
			stalled++
		}
		if lastDev >= 0 && dev-lastDev > 0.5 {
			t.Errorf("step %d: device Y jumped %.3f in one frame — Y is being snapped",
				i, dev-lastDev)
		}
		lastDev = dev
	}
	// Creeping an eighth of a pixel at 2x moves a quarter device pixel a frame,
	// so every frame must move. Any stall is the quantization coming back.
	if stalled > 0 {
		t.Errorf("%d frames did not move at all — text will step rather than glide", stalled)
	}
}

// Unhinted glyphs keep their sub-pixel position: above the hinting size limit
// the outline was never grid-fit, so snapping would only cost precision.
func TestUnhintedGlyphsKeepSubPixelPosition(t *testing.T) {
	_, _, fracX, _ := glyphPlacement(10.5, 40.25, 1, text.HintingNone)
	if fracX == 0 {
		t.Error("an unhinted glyph at x=10.5 should keep its 0.5 fraction")
	}
}

// The bug that X snapping originally caused: positions accumulated from
// *rounded advances* drifted a run off its measured width — 9px across 43
// characters at 9px. Rounding each absolute position independently cannot
// accumulate, so a long run stays within half a device pixel of exact.
func TestSnappingDoesNotAccumulateAcrossARun(t *testing.T) {
	const (
		advance = 5.37 // a deliberately unfriendly fractional advance
		n       = 200
		scale   = 2.0
	)
	for i := range n {
		exact := float64(i) * advance
		got, _, _, _ := glyphPlacement(exact, 0, scale, text.HintingFull)
		if err := math.Abs(got*scale - exact*scale); err > 0.5+1e-9 {
			t.Fatalf("glyph %d: placed %.4f, exact %.4f — off by %.4f device px; "+
				"error is accumulating rather than staying bounded",
				i, got*scale, exact*scale, err)
		}
	}
}

// A glyph must be rasterized at the size it is drawn at, not at the size it
// would be without a Transform.
//
// effectiveGlyphScale reads the whole matrix, which already folds the HiDPI
// scale in. Reading only deviceScale meant a widget scaled up by a Transform
// got a mask built at the untransformed size and then stretched by the GPU, so
// text inside a zoomed subtree went soft while everything around it stayed
// sharp.
func TestGlyphScaleFollowsTheWholeTransform(t *testing.T) {
	identity := gg.Matrix{A: 1, E: 1}
	if got := effectiveGlyphScale(identity, 1); got != 1 {
		t.Errorf("identity at 1x = %v, want 1", got)
	}

	// The HiDPI matrix is the transform for an untransformed widget, so this
	// must still be exactly deviceScale — the case that used to work.
	hidpi := gg.Matrix{A: 2, E: 2}
	if got := effectiveGlyphScale(hidpi, 2); got != 2 {
		t.Errorf("2x HiDPI, no user transform = %v, want 2", got)
	}

	// A widget scaled 1.8x on a 2x screen is drawn at 3.6 device px per em.
	scaled := gg.Matrix{A: 3.6, E: 3.6}
	if got := effectiveGlyphScale(scaled, 2); math.Abs(got-3.6) > 1e-9 {
		t.Errorf("1.8x under 2x HiDPI = %v, want 3.6", got)
	}

	// A rotation changes no size, so it must not change the strike either.
	const c, s = 0.7071067811865476, 0.7071067811865476
	rot := gg.Matrix{A: c, B: -s, D: s, E: c}
	if got := effectiveGlyphScale(rot, 1); math.Abs(got-1) > 1e-9 {
		t.Errorf("45° rotation = %v, want 1", got)
	}

	// Degenerate input falls back rather than asking for a zero-size mask.
	if got := effectiveGlyphScale(gg.Matrix{}, 2); got != 2 {
		t.Errorf("degenerate matrix = %v, want the deviceScale fallback 2", got)
	}

	// And an extreme zoom is bounded, so the atlas is never asked for an
	// enormous strike.
	if got := effectiveGlyphScale(gg.Matrix{A: 1000, E: 1000}, 2); got > 2*16 {
		t.Errorf("1000x = %v, want it clamped to 32", got)
	}
}
