package paint

import (
	"image"
	"testing"

	"github.com/doug/gophics/geom"
)

// plainCanvas hides every opt-in capability of the canvas it wraps: embedding
// the Canvas *interface* promotes only Canvas's own methods, so a plainCanvas
// is exactly what a backend that implements nothing but Canvas looks like —
// the scene recorder, or anyone else's. The paint package cannot import
// internal/scene (scene imports paint), so this stands in for it.
type plainCanvas struct{ Canvas }

// blendSurface paints an opaque destination, then src over it under mode, and
// returns the resulting pixel. The two colours are chosen so no channel pair
// is equal and none is 0 or 1, which is what makes a mode that silently falls
// back to Normal — the failure this whole table exists to catch — show up as a
// wrong number rather than a coincidence.
func blendSurface(t *testing.T, mode BlendMode) (r, g, b, a uint8) {
	t.Helper()
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 8, H: 8}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 8, 8), RGB(0.8, 0.6, 0.2)) // 204, 153, 51
	bl := Blends(c)
	if bl == nil {
		t.Fatal("the painter canvas does not implement Blender")
	}
	bl.PushBlend(mode)
	c.FillRect(geom.RectXYWH(0, 0, 8, 8), RGB(0.2, 0.4, 0.9)) // 51, 102, 229
	bl.PopBlend()
	return at8(p.SurfaceRGBA(), 4, 4)
}

func at8(img *image.RGBA, x, y int) (r, g, b, a uint8) {
	cr, cg, cb, ca := img.At(x, y).RGBA()
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)
}

// The point of exposing blend modes is that every one of them reaches the
// rasterizer. A mode that is accepted and then quietly composited normally is
// the worst outcome — it looks like it works until someone compares against
// Figma — and the substrate does have modes in that state on other code paths,
// so each one is pinned to the number the W3C formula gives for this pair of
// colours. Values are 8-bit because that is the space the compositing is done
// in, as the specification prescribes.
func TestEveryBlendModeComposites(t *testing.T) {
	const (
		dr, dg, db = 204, 153, 51 // destination
		sr, sg, sb = 51, 102, 229 // source
	)
	for _, tc := range []struct {
		mode       BlendMode
		r, g, b, a uint8
		why        string
	}{
		{BlendNormal, sr, sg, sb, 255, "opaque source over anything is the source"},
		{BlendMultiply, 41, 61, 46, 255, "d*s/255"},
		{BlendScreen, 214, 194, 234, 255, "255-(255-d)(255-s)/255"},
		{BlendOverlay, 173, 133, 92, 255, "hard light with the roles swapped"},
		{BlendDarken, 51, 102, 51, 255, "min per channel"},
		{BlendLighten, 204, 153, 229, 255, "max per channel"},
		{BlendDifference, 153, 51, 178, 255, "|d-s| per channel"},
		{BlendExclusion, 173, 133, 188, 255, "d+s-2ds/255"},
		{BlendPlus, 255, 255, 255, 255, "d+s clamped; this pair saturates"},
		{BlendModulate, 41, 61, 46, 255, "premultiplied multiply; opaque, so = multiply"},
		{BlendClear, 0, 0, 0, 0, "covered pixels become transparent"},
		{BlendSrc, sr, sg, sb, 255, "source replaces destination"},
		{BlendDst, dr, dg, db, 255, "destination survives untouched"},
		{BlendDstOver, dr, dg, db, 255, "opaque destination hides a source drawn behind it"},
		{BlendSrcIn, sr, sg, sb, 255, "destination is opaque everywhere, so all of the source shows"},
		{BlendDstIn, dr, dg, db, 255, "source is opaque everywhere, so all of the destination shows"},
		{BlendSrcOut, 0, 0, 0, 0, "nothing is outside an opaque destination"},
		{BlendDstOut, 0, 0, 0, 0, "nothing is outside an opaque source"},
		{BlendSrcAtop, sr, sg, sb, 255, "source clipped to an opaque destination is all of it"},
		{BlendDstAtop, dr, dg, db, 255, "destination clipped to an opaque source is all of it"},
		{BlendXor, 0, 0, 0, 0, "two opaque shapes overlap everywhere, so everything cancels"},
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			r, g, b, a := blendSurface(t, tc.mode)
			if r != tc.r || g != tc.g || b != tc.b || a != tc.a {
				t.Errorf("%v gave (%d,%d,%d,%d), want (%d,%d,%d,%d) — %s",
					tc.mode, r, g, b, a, tc.r, tc.g, tc.b, tc.a, tc.why)
			}
		})
	}
}

// luma is the Rec.601 luminosity the non-separable blend modes are defined
// against, in 8-bit units.
func luma(r, g, b uint8) float64 {
	return 0.3*float64(r) + 0.59*float64(g) + 0.11*float64(b)
}

// The four non-separable modes are the reason to have blend modes at all for
// theming: they recolour without disturbing shading. Their defining property
// is which of luminosity and chroma survives, so that is what is asserted —
// the exact channel values depend on the clipping the specification does when
// a recoloured pixel leaves gamut, which is not worth pinning.
func TestNonSeparableBlendModesKeepTheRightHalf(t *testing.T) {
	dstLuma := luma(204, 153, 51)
	srcLuma := luma(51, 102, 229)

	r, g, b, _ := blendSurface(t, BlendColor)
	if got := luma(r, g, b); got < dstLuma-2 || got > dstLuma+2 {
		t.Errorf("BlendColor luminosity %.1f, want the destination's %.1f — Color takes "+
			"the source's hue and saturation and must leave brightness alone", got, dstLuma)
	}
	r, g, b, _ = blendSurface(t, BlendLuminosity)
	if got := luma(r, g, b); got < srcLuma-2 || got > srcLuma+2 {
		t.Errorf("BlendLuminosity luminosity %.1f, want the source's %.1f", got, srcLuma)
	}
	// Hue and Saturation move the colour without being Normal; if they fell
	// back to Normal they would come out exactly the source.
	for _, m := range []BlendMode{BlendHue, BlendSaturation} {
		r, g, b, _ = blendSurface(t, m)
		if r == 51 && g == 102 && b == 229 {
			t.Errorf("%v produced the source unchanged — it fell back to Normal", m)
		}
	}
}

// The remaining separable modes are asymmetric in ways that are easy to get
// backwards in a mapping table, so each is pinned to the side it must favour.
func TestAsymmetricBlendModesPickTheRightSide(t *testing.T) {
	// ColorDodge brightens, ColorBurn darkens, and with this pair both
	// saturate — which is itself the check that they are not swapped.
	r, _, _, _ := blendSurface(t, BlendColorDodge)
	if r != 255 {
		t.Errorf("BlendColorDodge red %d, want 255 (dodge brightens to saturation here)", r)
	}
	r, g, _, _ := blendSurface(t, BlendColorBurn)
	if r != 0 || g != 0 {
		t.Errorf("BlendColorBurn red/green %d/%d, want 0/0 (burn darkens to black here)", r, g)
	}
	// HardLight is decided by the source, SoftLight by the destination, so
	// they must not agree.
	hr, hg, hb, _ := blendSurface(t, BlendHardLight)
	sr, sg, sb, _ := blendSurface(t, BlendSoftLight)
	if hr == sr && hg == sg && hb == sb {
		t.Error("HardLight and SoftLight produced the same pixel — one of them is mismapped")
	}
}

// Blend mode is state, so the thing that has to be true is that it nests: an
// inner mode wins while it is pushed and the outer one comes back afterwards.
// A widget sets a mode around its own subtree without knowing or restoring
// what its parent did, so a stack that leaked would tint everything drawn
// after it for the rest of the frame.
func TestBlendModeNestsAndRestores(t *testing.T) {
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 30, H: 10}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 30, 10), RGB(0.8, 0.6, 0.2))
	b := Blends(c)

	b.PushBlend(BlendMultiply)
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.2, 0.4, 0.9)) // multiplied
	b.PushBlend(BlendDst)
	c.FillRect(geom.RectXYWH(10, 0, 10, 10), RGB(0.2, 0.4, 0.9)) // discarded
	b.PopBlend()
	c.FillRect(geom.RectXYWH(20, 0, 10, 10), RGB(0.2, 0.4, 0.9)) // multiplied again
	b.PopBlend()

	img := p.SurfaceRGBA()
	outer := [3]uint8{41, 61, 46} // dst*src
	if r, g, bl, _ := at8(img, 5, 5); r != outer[0] || g != outer[1] || bl != outer[2] {
		t.Errorf("outer multiply gave (%d,%d,%d), want %v", r, g, bl, outer)
	}
	if r, g, bl, _ := at8(img, 15, 5); r != 204 || g != 153 || bl != 51 {
		t.Errorf("inner BlendDst gave (%d,%d,%d), want the untouched destination (204,153,51)", r, g, bl)
	}
	if r, g, bl, _ := at8(img, 25, 5); r != outer[0] || g != outer[1] || bl != outer[2] {
		t.Errorf("after the inner pop the outer multiply did not resume: (%d,%d,%d), want %v", r, g, bl, outer)
	}
}

// Blend mode lives in gg's paint state, which gg's own Push/Pop (used by
// PushClip and PushTransform) deliberately does not save or restore. That is
// why ggCanvas keeps the stack itself, and this is the test that notices if
// someone ever decides to lean on gg's save/restore instead.
func TestBlendModeSurvivesClipsAndTransforms(t *testing.T) {
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.8, 0.6, 0.2))
	b := Blends(c)

	b.PushBlend(BlendMultiply)
	c.PushClip(geom.RectXYWH(0, 0, 10, 10))
	c.PushTransform(Transform{})
	c.PopTransform()
	c.PopClip()
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.2, 0.4, 0.9))
	b.PopBlend()

	if r, g, bl, _ := at8(p.SurfaceRGBA(), 5, 5); r != 41 || g != 61 || bl != 46 {
		t.Errorf("got (%d,%d,%d), want the multiplied (41,61,46) — a clip or transform "+
			"push/pop reset the blend mode", r, g, bl)
	}
}

// An unbalanced PopBlend is a bug in the caller either way, but the two ways
// to react are not equally bad: resetting to Normal spoils one shape, while
// panicking takes the frame down and leaving the mode set spoils every shape
// drawn for the rest of it.
func TestUnbalancedPopBlendResetsToNormal(t *testing.T) {
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.8, 0.6, 0.2))
	b := Blends(c)
	b.PushBlend(BlendMultiply)
	b.PopBlend()
	b.PopBlend() // one too many
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.2, 0.4, 0.9))

	if r, g, bl, _ := at8(p.SurfaceRGBA(), 5, 5); r != 51 || g != 102 || bl != 229 {
		t.Errorf("got (%d,%d,%d), want the plain source (51,102,229)", r, g, bl)
	}
}

// nil from Blends is a real answer, not an oversight: a Canvas that only
// records drawing has no destination to blend against, and a caller that
// needs blending has to be able to tell rather than discover it from a
// screenshot.
func TestCanvasWithoutTheCapabilityReportsNil(t *testing.T) {
	p := NewPainter()
	if Blends(plainCanvas{p.BeginOffscreen(geom.Size{W: 4, H: 4}, 1)}) != nil {
		t.Error("Blends found a Blender on a canvas that implements only Canvas")
	}
	if Blends(p.BeginOffscreen(geom.Size{W: 4, H: 4}, 1)) == nil {
		t.Error("Blends found no Blender on the painter canvas, which does blend")
	}
}

// A mode's name shows up in golden-test failures and diagnostics, so every one
// of them needs a distinct one — and a value past the end must not index out
// of the table.
func TestBlendModeNames(t *testing.T) {
	seen := map[string]BlendMode{}
	for m := BlendNormal; m < numBlendModes; m++ {
		n := m.String()
		if n == "" {
			t.Errorf("mode %d has no name", m)
		}
		if prev, dup := seen[n]; dup {
			t.Errorf("modes %d and %d are both called %q", prev, m, n)
		}
		seen[n] = m
	}
	if got := BlendMode(200).String(); got != "BlendMode(?)" {
		t.Errorf("out-of-range mode named %q", got)
	}
}

// A stray mode — interpolated, deserialised, or simply miscast — must paint
// plainly rather than take the frame down.
func TestUnknownBlendModePaintsNormally(t *testing.T) {
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.8, 0.6, 0.2))
	b := Blends(c)
	b.PushBlend(BlendMode(200))
	c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.2, 0.4, 0.9))
	b.PopBlend()
	if r, g, bl, _ := at8(p.SurfaceRGBA(), 5, 5); r != 51 || g != 102 || bl != 229 {
		t.Errorf("got (%d,%d,%d), want the plain source (51,102,229)", r, g, bl)
	}
}

// Blend modes have to behave the same at 2×, where the shape covers four
// times the pixels and the layer machinery underneath allocates at physical
// resolution — a place this repository has shipped a HiDPI bug before.
func TestBlendModeAtDevicePixelRatioTwo(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		p := NewPainter()
		c := p.BeginOffscreen(geom.Size{W: 10, H: 10}, scale)
		c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.8, 0.6, 0.2))
		b := Blends(c)
		b.PushBlend(BlendMultiply)
		c.FillRect(geom.RectXYWH(0, 0, 10, 10), RGB(0.2, 0.4, 0.9))
		b.PopBlend()
		img := p.SurfaceRGBA()
		// The bottom-right corner is the one a logical-vs-physical mix-up
		// leaves unpainted.
		x, y := img.Bounds().Max.X-2, img.Bounds().Max.Y-2
		if r, g, bl, _ := at8(img, x, y); r != 41 || g != 61 || bl != 46 {
			t.Errorf("scale %v at (%d,%d): (%d,%d,%d), want the multiplied (41,61,46)",
				scale, x, y, r, g, bl)
		}
	}
}
