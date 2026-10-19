package paint

import (
	"image"
	"math"
	"testing"

	"github.com/doug/gophics/geom"
)

func nearly(a, b, tol float32) bool {
	d := a - b
	return d > -tol && d < tol
}

func sameColor(a, b Color, tol float32) bool {
	return nearly(a.R, b.R, tol) && nearly(a.G, b.G, tol) &&
		nearly(a.B, b.B, tol) && nearly(a.A, b.A, tol)
}

var (
	red   = RGB(1, 0, 0)
	green = RGB(0, 1, 0)
	blue  = RGB(0, 0, 1)
	white = RGB(1, 1, 1)
	black = RGB(0, 0, 0)
	cyan  = RGB(0, 1, 1)
)

// A ramp has to hit each stop's colour exactly at that stop's position: a
// caller picks those colours deliberately, and a gradient that only comes
// close to them makes a design system's tokens unverifiable.
func TestGradientIsExactAtEveryStop(t *testing.T) {
	g := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, red}, GradientStop{0.3, green},
		GradientStop{0.8, blue}, GradientStop{1, white})
	for _, s := range g.stops {
		if got := g.At(s.Pos); got != s.Color {
			t.Errorf("At(%v) = %+v, want the stop's own %+v", s.Pos, got, s.Color)
		}
	}
	// Past either end the ramp pads rather than extrapolating or repeating.
	if got := g.At(-5); got != red {
		t.Errorf("At(-5) = %+v, want the first stop %+v", got, red)
	}
	if got := g.At(5); got != white {
		t.Errorf("At(5) = %+v, want the last stop %+v", got, white)
	}
}

// Between stops, a multi-stop ramp has to interpolate within the segment the
// position falls in and not across the whole ramp — the mistake that makes a
// three-stop gradient look like a two-stop one with the middle colour showing
// only at one pixel.
func TestGradientInterpolatesWithinItsSegment(t *testing.T) {
	g := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, black}, GradientStop{0.5, red}, GradientStop{1, white})

	// A quarter of the way along is the middle of the black→red segment, so
	// it must be a dark red: red rising, green and blue still at zero.
	q := g.At(0.25)
	if q.G != 0 || q.B != 0 {
		t.Errorf("At(0.25) = %+v, want green and blue still at zero inside the black→red segment", q)
	}
	if q.R <= 0 || q.R >= 1 {
		t.Errorf("At(0.25) red %v, want strictly between the segment's ends", q.R)
	}
	// Three quarters is the middle of red→white, so green and blue must have
	// left zero while red stays at the top.
	h := g.At(0.75)
	if h.G <= 0 || h.B <= 0 {
		t.Errorf("At(0.75) = %+v, want green and blue rising inside the red→white segment", h)
	}
	if !nearly(h.R, 1, 1e-3) {
		t.Errorf("At(0.75) red %v, want 1 — both ends of red→white have full red", h.R)
	}
}

// Stops may be written in any order. Sorting them is easy to get wrong in the
// direction that matters least and right in the direction that matters most,
// so both are checked: order-independence, and *stability* at equal positions.
func TestGradientStopsAreSortedStably(t *testing.T) {
	shuffled := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{1, blue}, GradientStop{0, red}, GradientStop{0.5, green})
	for _, tc := range []struct {
		t    float32
		want Color
	}{{0, red}, {0.5, green}, {1, blue}} {
		if got := shuffled.At(tc.t); got != tc.want {
			t.Errorf("At(%v) = %+v, want %+v — stops given out of order were not sorted", tc.t, got, tc.want)
		}
	}

	// Out-of-range positions clamp rather than being dropped, so a stop at
	// 1.5 still terminates the ramp.
	clamped := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{-1, red}, GradientStop{1.5, blue})
	if got := clamped.At(0); got != red {
		t.Errorf("At(0) = %+v, want %+v — a stop at -1 should clamp to 0", got, red)
	}
	if got := clamped.At(1); got != blue {
		t.Errorf("At(1) = %+v, want %+v — a stop at 1.5 should clamp to 1", got, blue)
	}
}

// Two stops at one position are the idiom for a hard edge — a two-tone band, a
// pie slice boundary — and which colour lands on which side of it is decided
// by the order they were written. A sort that reordered equal keys would
// silently flip every such edge.
func TestDuplicateStopPositionsMakeAHardEdge(t *testing.T) {
	g := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, red}, GradientStop{0.5, green},
		GradientStop{0.5, blue}, GradientStop{1, white})

	// Just below the doubled position the first segment is still running and
	// has nearly reached its end colour. "Nearly" is generous on the channel
	// that is on its way to zero: in linear light the sRGB encoding's slope
	// near black is almost thirteen to one, so the last thousandth of the
	// segment still has a visible amount of red left in it.
	if got := g.At(0.4999); got.G < 0.99 || got.R > 0.02 || got.B != 0 {
		t.Errorf("At(0.4999) = %+v, want essentially %+v (the end of the first segment)", got, green)
	}
	if got := g.At(0.5); got != blue {
		t.Errorf("At(0.5) = %+v, want %+v — at a doubled position the later stop takes over", got, blue)
	}
	if got := g.At(0.5001); got.B <= 0.9 || got.R <= 0 {
		t.Errorf("At(0.5001) = %+v, want it heading from blue towards white", got)
	}

	// Three at once, and a doubled position at the very start.
	edge := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, red}, GradientStop{0, blue}, GradientStop{1, white})
	if got := edge.At(0); got != red {
		t.Errorf("At(0) = %+v, want the first-written %+v", got, red)
	}
	if got := edge.At(0.001); got.B <= 0.9 {
		t.Errorf("At(0.001) = %+v, want it starting from blue", got)
	}
}

// Degenerate ramps must paint something defensible rather than dividing by
// zero: a gradient arrives from theme data and animation, and an empty stop
// list is a perfectly ordinary intermediate state.
func TestDegenerateGradients(t *testing.T) {
	if got := LinearGradient(geom.Pt{}, geom.Pt{X: 10}).At(0.5); got != (Color{}) {
		t.Errorf("a gradient with no stops gave %+v, want transparent", got)
	}
	flat := LinearGradient(geom.Pt{}, geom.Pt{X: 10}, GradientStop{0.3, green})
	for _, tt := range []float32{-1, 0, 0.3, 1, 2} {
		if got := flat.At(tt); got != green {
			t.Errorf("one-stop gradient At(%v) = %+v, want %+v everywhere", tt, got, green)
		}
	}
	// A zero-length axis, a zero radius and a zero sweep all have no ramp to
	// walk, so every point is position 0.
	for _, g := range []*Gradient{
		LinearGradient(geom.Pt{X: 5, Y: 5}, geom.Pt{X: 5, Y: 5}, GradientStop{0, red}, GradientStop{1, blue}),
		RadialGradient(geom.Pt{X: 5, Y: 5}, 0, GradientStop{0, red}, GradientStop{1, blue}),
		SweepGradient(geom.Pt{X: 5, Y: 5}, 1, 1, GradientStop{0, red}, GradientStop{1, blue}),
	} {
		if got := g.Param(geom.Pt{X: 9, Y: 2}); got != 0 {
			t.Errorf("degenerate geometry gave Param = %v, want 0", got)
		}
	}
}

// Interpolating the sRGB-encoded numbers directly walks through a dead zone:
// two complementary colours average to a dark muddy grey rather than to the
// light they actually sum to. This is the test that pins the interpolation to
// linear light — and it fails loudly if anyone "simplifies" At back to a
// component lerp, because the two answers differ by nearly a quarter of the
// range.
func TestGradientInterpolatesInLinearLight(t *testing.T) {
	g := LinearGradient(geom.Pt{}, geom.Pt{X: 100}, GradientStop{0, red}, GradientStop{1, cyan})
	mid := g.At(0.5)

	// Half the light of full red plus half of full cyan is 0.5 in linear
	// light on every channel, which encodes to about 0.7354 in sRGB.
	const want = 0.7354
	for _, ch := range []struct {
		name string
		v    float32
	}{{"R", mid.R}, {"G", mid.G}, {"B", mid.B}} {
		if !nearly(ch.v, want, 0.003) {
			t.Errorf("red→cyan midpoint %s = %v, want ≈%v (linear light)", ch.name, ch.v, want)
		}
	}
	// And specifically not the sRGB-space answer, which is the bug.
	if nearly(mid.R, 0.5, 0.05) {
		t.Errorf("red→cyan midpoint is %v — that is the naive sRGB lerp's muddy grey", mid.R)
	}

	// Black to white is where the two spaces disagree most visibly: the
	// linear-light midpoint is a bright mid grey, not 50%% grey.
	bw := LinearGradient(geom.Pt{}, geom.Pt{X: 100}, GradientStop{0, black}, GradientStop{1, white})
	if m := bw.At(0.5).R; !nearly(m, want, 0.003) {
		t.Errorf("black→white midpoint %v, want ≈%v", m, want)
	}
}

// Interpolation is premultiplied, which is a separate claim from linear light
// and has its own failure: a stop's colour means nothing where its alpha is
// zero, so interpolating the straight components drags the ramp towards
// whatever that stop happens to name — black, in every fade-to-nothing anyone
// writes — and the midpoint comes out a dark half-transparent smear.
func TestFadeToTransparentKeepsItsColor(t *testing.T) {
	g := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, red}, GradientStop{1, Color{}}) // transparent black
	mid := g.At(0.5)

	if !nearly(mid.A, 0.5, 1e-4) {
		t.Errorf("alpha %v at the midpoint, want 0.5 — alpha is coverage and stays linear", mid.A)
	}
	if !nearly(mid.R, 1, 1e-3) {
		t.Errorf("red %v at the midpoint, want 1 — premultiplied interpolation keeps the hue "+
			"all the way down to nothing; straight interpolation would darken it", mid.R)
	}
	if mid.G != 0 || mid.B != 0 {
		t.Errorf("midpoint %+v picked up green or blue from a stop with no alpha", mid)
	}
	// Fully transparent at both ends has no hue to report, and must not be a
	// NaN from dividing out a zero alpha.
	none := LinearGradient(geom.Pt{}, geom.Pt{X: 100},
		GradientStop{0, Color{}}, GradientStop{1, Color{R: 1}})
	if m := none.At(0); m != (Color{}) {
		t.Errorf("At(0) of a transparent stop = %+v, want the zero Color", m)
	}
	for _, v := range []float32{none.At(0.5).R, none.At(0.5).A} {
		if math.IsNaN(float64(v)) {
			t.Error("interpolating through zero alpha produced a NaN")
		}
	}
}

// Param is the geometry half of a gradient, and each kind has a different way
// to get it wrong: a linear ramp that varies across its axis instead of along
// it, a radial one scaled by diameter rather than radius, a sweep that starts
// wherever atan2's branch cut happens to fall.
func TestParamGeometry(t *testing.T) {
	lin := LinearGradient(geom.Pt{X: 10, Y: 10}, geom.Pt{X: 110, Y: 10})
	for _, tc := range []struct {
		p    geom.Pt
		want float32
		why  string
	}{
		{geom.Pt{X: 10, Y: 10}, 0, "the start point"},
		{geom.Pt{X: 60, Y: 10}, 0.5, "halfway along"},
		{geom.Pt{X: 110, Y: 10}, 1, "the end point"},
		{geom.Pt{X: 60, Y: 900}, 0.5, "moving across the axis must not change the position"},
		{geom.Pt{X: -40, Y: 10}, -0.5, "before the start, so negative"},
	} {
		if got := lin.Param(tc.p); !nearly(got, tc.want, 1e-5) {
			t.Errorf("linear Param%v = %v, want %v — %s", tc.p, got, tc.want, tc.why)
		}
	}

	rad := RadialGradient(geom.Pt{X: 50, Y: 50}, 20)
	for _, tc := range []struct {
		p    geom.Pt
		want float32
	}{
		{geom.Pt{X: 50, Y: 50}, 0},
		{geom.Pt{X: 60, Y: 50}, 0.5},
		{geom.Pt{X: 50, Y: 30}, 1},
		{geom.Pt{X: 90, Y: 50}, 2},
	} {
		if got := rad.Param(tc.p); !nearly(got, tc.want, 1e-5) {
			t.Errorf("radial Param%v = %v, want %v", tc.p, got, tc.want)
		}
	}

	// Angle 0 is +X and angles increase clockwise, because y grows downward.
	sweep := SweepGradient(geom.Pt{}, 0, 2*math.Pi)
	for _, tc := range []struct {
		p    geom.Pt
		want float32
	}{
		{geom.Pt{X: 1}, 0},
		{geom.Pt{Y: 1}, 0.25},
		{geom.Pt{X: -1}, 0.5},
		{geom.Pt{Y: -1}, 0.75},
	} {
		if got := sweep.Param(tc.p); !nearly(got, tc.want, 1e-5) {
			t.Errorf("sweep Param%v = %v, want %v (clockwise from +X)", tc.p, got, tc.want)
		}
	}
	// A rotated start angle moves the ramp with it rather than clipping it.
	rotated := SweepGradient(geom.Pt{}, math.Pi/2, math.Pi/2+2*math.Pi)
	if got := rotated.Param(geom.Pt{Y: 1}); !nearly(got, 0, 1e-5) {
		t.Errorf("rotated sweep Param at its start angle = %v, want 0", got)
	}
	if got := rotated.Param(geom.Pt{X: -1}); !nearly(got, 0.25, 1e-5) {
		t.Errorf("rotated sweep Param a quarter turn on = %v, want 0.25", got)
	}
	// A negative sweep runs the other way around.
	back := SweepGradient(geom.Pt{}, 0, -2*math.Pi)
	if got := back.Param(geom.Pt{Y: -1}); !nearly(got, 0.25, 1e-5) {
		t.Errorf("anticlockwise sweep Param = %v, want 0.25", got)
	}
	// The centre has no angle; it must answer rather than produce a NaN.
	if got := sweep.Param(geom.Pt{}); got != 0 {
		t.Errorf("sweep Param at its own centre = %v, want 0", got)
	}
}

func TestExtentIsTheRampsLength(t *testing.T) {
	r := geom.RectXYWH(0, 0, 100, 100)
	if got := LinearGradient(geom.Pt{}, geom.Pt{X: 30, Y: 40}).Extent(r); !nearly(got, 50, 1e-4) {
		t.Errorf("linear Extent = %v, want the axis length 50", got)
	}
	if got := RadialGradient(geom.Pt{X: 50, Y: 50}, 20).Extent(r); got != 20 {
		t.Errorf("radial Extent = %v, want the radius 20", got)
	}
	// A sweep's ramp is stretched around the furthest corner it has to cover.
	want := float32(2 * math.Pi * math.Hypot(50, 50))
	if got := SweepGradient(geom.Pt{X: 50, Y: 50}, 0, 2*math.Pi).Extent(r); !nearly(got, want, 0.01) {
		t.Errorf("sweep Extent = %v, want %v (one turn at the corner radius)", got, want)
	}
}

// The band fallback's quality is entirely decided by how many bands it uses,
// and a fixed count is wrong in both directions at once. These are the two
// limits that replaced it.
func TestBandCountAdaptsToExtentAndRamp(t *testing.T) {
	violent := EvenStops(red, cyan) // a full-range channel swing
	r := geom.RectXYWH(0, 0, 400, 40)

	// Extent-limited: a ramp cannot show more steps than it has pixels, and
	// the pixels are *physical*, so 2× needs twice as many bands. Getting
	// this in logical pixels is how a gradient comes out smooth on a 1×
	// development machine and striped on a Retina one.
	small := LinearGradient(geom.Pt{}, geom.Pt{X: 40}, violent...)
	if got := gradientBandCount(small, r, 1); got != 40 {
		t.Errorf("40 logical px at 1× gave %d bands, want 40 (one per device pixel)", got)
	}
	if got := gradientBandCount(small, r, 2); got != 80 {
		t.Errorf("40 logical px at 2× gave %d bands, want 80 — the limit is in physical pixels", got)
	}

	// Ramp-limited: a wash between two nearly equal colours needs a handful
	// of bands however much room it is given, where the old fixed 64 spent
	// the same effort on it as on a black-to-white sweep.
	subtle := LinearGradient(geom.Pt{}, geom.Pt{X: 400},
		GradientStop{0, RGB(0.5, 0.5, 0.5)}, GradientStop{1, RGB(0.52, 0.52, 0.52)})
	if got := gradientBandCount(subtle, r, 2); got > 8 {
		t.Errorf("a 2%%-of-range wash over 400 px gave %d bands, want a handful — "+
			"the step is already under one 8-bit level", got)
	}
	// ... and a full-range one over the same room needs hundreds.
	wide := LinearGradient(geom.Pt{}, geom.Pt{X: 400}, violent...)
	if got := gradientBandCount(wide, r, 2); got < 200 {
		t.Errorf("a full-range ramp over 400 px gave only %d bands", got)
	}

	// Both ends are bounded. Nothing above 256 can improve an 8-bit ramp, and
	// on a recorder each band is also a display-list op.
	zigzag := LinearGradient(geom.Pt{}, geom.Pt{X: 4000}, EvenStops(black, white, black, white)...)
	if got := gradientBandCount(zigzag, geom.RectXYWH(0, 0, 4000, 40), 2); got != maxGradientBands {
		t.Errorf("a ramp needing %d steps gave %d bands, want the cap %d",
			zigzag.rampSteps(), got, maxGradientBands)
	}
	flat := LinearGradient(geom.Pt{}, geom.Pt{X: 0}, violent...)
	if got := gradientBandCount(flat, r, 1); got != minGradientBands {
		t.Errorf("a zero-extent gradient gave %d bands, want the floor %d", got, minGradientBands)
	}
}

// surfaceFor paints g over the whole of a logical size at the given device
// scale and hands back the physical pixmap.
func surfaceFor(scale float32, size geom.Size, wrap bool, g *Gradient) *image.RGBA {
	p := NewPainter()
	var c Canvas = p.BeginOffscreen(size, scale)
	c.Clear(RGB(0, 1, 0)) // a colour no test gradient uses, so gaps show
	if wrap {
		c = plainCanvas{c}
	}
	FillGradient(c, geom.Rect{Max: geom.Pt{X: size.W, Y: size.H}}, 0, g)
	return p.SurfaceRGBA()
}

// checkAgainstRamp walks every device pixel of a row (or column) and compares
// it to the colour the gradient says belongs at that pixel's centre, mapped
// back to logical coordinates. This is the assertion that a gradient is right
// rather than merely smooth, and it is written in terms of the device scale so
// that it is the *same* assertion at 1× and 2×.
func checkAgainstRamp(t *testing.T, name string, img *image.RGBA, scale float32, g *Gradient, tol int, vertical bool) {
	t.Helper()
	b := img.Bounds()
	n, fixed := b.Dx(), b.Dy()/2
	if vertical {
		n, fixed = b.Dy(), b.Dx()/2
	}
	worst := 0
	for i := 2; i < n-2; i++ { // skip the anti-aliased outer edge
		x, y := i, fixed
		if vertical {
			x, y = fixed, i
		}
		lx := (float32(x) + 0.5) / scale
		ly := (float32(y) + 0.5) / scale
		want := g.At(g.Param(geom.Pt{X: lx, Y: ly}))
		gr, gg8, gb, _ := at8(img, x, y)
		for _, d := range []int{
			int(gr) - int(clamp01(want.R)*255),
			int(gg8) - int(clamp01(want.G)*255),
			int(gb) - int(clamp01(want.B)*255),
		} {
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst = d
			}
		}
		if worst > tol {
			t.Fatalf("%s at scale %v: device pixel (%d,%d) is (%d,%d,%d) but the ramp says "+
				"(%d,%d,%d) — off by %d levels (tolerance %d)", name, scale, x, y,
				gr, gg8, gb, int(want.R*255), int(want.G*255), int(want.B*255), worst, tol)
		}
	}
	t.Logf("%s at scale %v: worst channel error %d of %d allowed", name, scale, worst, tol)
}

// The documented HiDPI trap, in the place it actually bit: gg samples a brush
// with physical pixel coordinates straight off the pixmap, while a gradient's
// geometry is in logical pixels. Handing it logical endpoints made the ramp
// finish a quarter of the way across the shape at 2× and left the rest flat —
// correct at 1×, wrong on every Retina screen.
func TestGradientFollowsTheRampAtEveryScale(t *testing.T) {
	size := geom.Size{W: 60, H: 20}
	for _, scale := range []float32{1, 2, 3} {
		h := LinearGradient(geom.Pt{}, geom.Pt{X: 60}, EvenStops(red, green, blue)...)
		checkAgainstRamp(t, "horizontal 3-stop", surfaceFor(scale, size, false, h), scale, h, 2, false)

		v := LinearGradient(geom.Pt{}, geom.Pt{Y: 20}, EvenStops(white, blue)...)
		checkAgainstRamp(t, "vertical", surfaceFor(scale, size, false, v), scale, v, 2, true)

		d := LinearGradient(geom.Pt{}, geom.Pt{X: 60, Y: 20}, EvenStops(red, cyan)...)
		checkAgainstRamp(t, "diagonal", surfaceFor(scale, size, false, d), scale, d, 2, false)

		r := RadialGradient(geom.Pt{X: 30, Y: 10}, 25, EvenStops(white, blue)...)
		checkAgainstRamp(t, "radial", surfaceFor(scale, size, false, r), scale, r, 2, false)

		s := SweepGradient(geom.Pt{X: 30, Y: 10}, 0, 2*math.Pi, EvenStops(red, green, blue, red)...)
		checkAgainstRamp(t, "sweep", surfaceFor(scale, size, false, s), scale, s, 3, false)
	}
}

// The band fallback is what a Canvas without a gradient brush gets — the GPU
// tile rasterizer, and the scene recorder. It has to track the same ramp, to
// within the step the adaptive band count allows, at every scale: a fallback
// that is right at 1× and shifted at 2× is the same bug one layer down.
func TestBandFallbackFollowsTheSameRamp(t *testing.T) {
	size := geom.Size{W: 60, H: 20}
	for _, scale := range []float32{1, 2} {
		h := LinearGradient(geom.Pt{}, geom.Pt{X: 60}, EvenStops(red, green, blue)...)
		checkAgainstRamp(t, "banded horizontal", surfaceFor(scale, size, true, h), scale, h, 6, false)

		v := LinearGradient(geom.Pt{}, geom.Pt{Y: 20}, EvenStops(white, blue)...)
		checkAgainstRamp(t, "banded vertical", surfaceFor(scale, size, true, v), scale, v, 6, true)

		d := LinearGradient(geom.Pt{}, geom.Pt{X: 60, Y: 20}, EvenStops(red, cyan)...)
		checkAgainstRamp(t, "banded diagonal", surfaceFor(scale, size, true, d), scale, d, 14, false)

		r := RadialGradient(geom.Pt{X: 30, Y: 10}, 25, EvenStops(white, blue)...)
		checkAgainstRamp(t, "banded radial", surfaceFor(scale, size, true, r), scale, r, 8, false)
	}
}

// A sweep gradient's bands are wedges, so how closely they track the ramp
// depends on where you stand: near the centre a single pixel spans a wide
// angle and covers several wedges at once, and no wedge count fixes that — the
// exact brush has the same problem, it just hides it by sampling one point.
// So the rim is where the approximation is checked, sampled all the way round
// rather than along a row, which is also the only way to catch a wedge fan
// that has drifted in angle.
func TestBandedSweepFollowsTheRampAroundItsRim(t *testing.T) {
	const side = 80
	for _, scale := range []float32{1, 2} {
		center := geom.Pt{X: side / 2, Y: side / 2}
		g := SweepGradient(center, 0, 2*math.Pi, EvenStops(red, green, blue, red)...)
		img := surfaceFor(scale, geom.Size{W: side, H: side}, true, g)
		radius := float32(side/2 - 8)
		worst, worstAng := 0, float32(0)
		for i := range 180 {
			ang := float32(i) * 2 * math.Pi / 180
			lx := center.X + radius*float32(math.Cos(float64(ang)))
			ly := center.Y + radius*float32(math.Sin(float64(ang)))
			x, y := int(lx*scale), int(ly*scale)
			want := g.At(g.Param(geom.Pt{X: (float32(x) + 0.5) / scale, Y: (float32(y) + 0.5) / scale}))
			gr, gg8, gb, _ := at8(img, x, y)
			for _, d := range []int{
				int(gr) - int(clamp01(want.R)*255),
				int(gg8) - int(clamp01(want.G)*255),
				int(gb) - int(clamp01(want.B)*255),
			} {
				if d < 0 {
					d = -d
				}
				if d > worst {
					worst, worstAng = d, ang
				}
			}
		}
		t.Logf("banded sweep rim at scale %v: worst channel error %d", scale, worst)
		if worst > 10 {
			t.Errorf("banded sweep at scale %v is off by %d levels at %.2f rad — the wedge fan "+
				"does not track the ramp", scale, worst, worstAng)
		}
	}
}

// The band fallback must leave no gaps, and "no gaps" has to mean no *partial*
// gaps either: a seam only one anti-aliased edge wide still shows as a
// hairline, and a wedge fan whose seams all point at the same centre shows it
// as a starburst. Every test gradient here runs between red and blue, and the
// surface is cleared to green, so any green at all in a pixel is backdrop
// leaking through a seam — a far sharper instrument than looking for pixels
// that are *entirely* background.
func TestBandFallbackCoversEveryPixel(t *testing.T) {
	size := geom.Size{W: 50, H: 50}
	for _, tc := range []struct {
		name string
		g    *Gradient
	}{
		{"horizontal", LinearGradient(geom.Pt{X: 10}, geom.Pt{X: 40}, EvenStops(red, blue)...)},
		{"vertical", LinearGradient(geom.Pt{Y: 10}, geom.Pt{Y: 40}, EvenStops(red, blue)...)},
		{"diagonal", LinearGradient(geom.Pt{X: 10, Y: 10}, geom.Pt{X: 40, Y: 40}, EvenStops(red, blue)...)},
		{"radial", RadialGradient(geom.Pt{X: 25, Y: 25}, 15, EvenStops(red, blue)...)},
		{"sweep full", SweepGradient(geom.Pt{X: 25, Y: 25}, 0, 2*math.Pi, EvenStops(red, blue, red)...)},
		{"sweep partial", SweepGradient(geom.Pt{X: 25, Y: 25}, 0.3, 2.5, EvenStops(red, blue)...)},
		// A gradient defined once in a parent's coordinates and then used on
		// a child can easily be anchored outside the shape it fills, which is
		// the case a reach computed from the shape's own diagonal misses.
		{"axis anchored outside", LinearGradient(
			geom.Pt{X: -200, Y: -200}, geom.Pt{X: -150, Y: -150}, EvenStops(red, blue)...)},
		{"oblique axis anchored outside", LinearGradient(
			geom.Pt{X: -60, Y: 10}, geom.Pt{X: -10, Y: 60}, EvenStops(red, blue)...)},
		{"radial centred outside", RadialGradient(geom.Pt{X: -30, Y: -30}, 40, EvenStops(red, blue)...)},
		{"sweep centred outside", SweepGradient(
			geom.Pt{X: -20, Y: 25}, 0, 2*math.Pi, EvenStops(red, blue, red)...)},
	} {
		img := surfaceFor(2, size, true, tc.g)
		b := img.Bounds()
		worst, wx, wy := uint8(0), 0, 0
		for y := b.Min.Y + 1; y < b.Max.Y-1; y++ {
			for x := b.Min.X + 1; x < b.Max.X-1; x++ {
				if _, g, _, _ := at8(img, x, y); g > worst {
					worst, wx, wy = g, x, y
				}
			}
		}
		if worst > 12 {
			t.Errorf("%s: the green backdrop shows through at %d/255 at device pixel (%d,%d) — "+
				"a seam between bands", tc.name, worst, wx, wy)
		}
		t.Logf("%s: worst backdrop leak %d/255", tc.name, worst)
	}
}

// worstNeighbourStep is the largest jump between horizontally neighbouring
// device pixels along the middle row — the thing an eye reads as a band.
func worstNeighbourStep(img *image.RGBA) (worst, at int) {
	b := img.Bounds()
	y := b.Dy() / 2
	for x := b.Min.X + 2; x < b.Max.X-2; x++ {
		pr, pg, pb, _ := at8(img, x-1, y)
		cr, cg, cb, _ := at8(img, x, y)
		for _, d := range []int{int(cr) - int(pr), int(cg) - int(pg), int(cb) - int(pb)} {
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst, at = d, x
			}
		}
	}
	return worst, at
}

// Banding is the failure the adaptive band count exists to avoid, and the
// claim it makes is a claim about *scaling*: stretching a ramp over four times
// the room must not make the steps four times bigger, which is exactly what a
// fixed count of 64 strips does. So the same ramp is measured small and large,
// and both have to stay smooth.
//
// The ramp is a mid-tone one on purpose. A ramp that reaches pure black has an
// unavoidably steep toe — in linear light the sRGB encoding's slope there is
// nearly thirteen to one, so even the exact per-pixel brush steps by three
// levels at a time near the dark end, and a threshold that tripped on that
// would be measuring 8-bit quantisation rather than banding.
func TestBandCountKeepsLargeGradientsSmooth(t *testing.T) {
	stops := EvenStops(RGB(0.25, 0.35, 0.55), RGB(0.9, 0.8, 0.65))
	for _, width := range []float32{150, 600} {
		g := LinearGradient(geom.Pt{}, geom.Pt{X: width}, stops...)
		size := geom.Size{W: width, H: 16}
		brush, _ := worstNeighbourStep(surfaceFor(2, size, false, g))
		bands, at := worstNeighbourStep(surfaceFor(2, size, true, g))
		t.Logf("%.0f logical px: brush steps by %d, bands by %d (%d bands)",
			width, brush, bands, gradientBandCount(g, geom.Rect{Max: geom.Pt{X: width, Y: 16}}, 2))
		if bands > 3 {
			t.Errorf("over %.0f px the band route jumps %d levels at x=%d — a visible band",
				width, bands, at)
		}
		if brush > 2 {
			t.Errorf("over %.0f px the exact brush jumps %d levels, which should not happen "+
				"on a mid-tone ramp", width, brush)
		}
	}
}

// Canvas.FillRRectGradient is the two-stop call the scene recorder records as
// a single op, so it has to stay — but it is now just a Gradient, and the two
// spellings must paint the same pixels or a recorded frame and a directly
// drawn one would differ.
func TestFillRRectGradientIsAnAxisGradient(t *testing.T) {
	for _, horizontal := range []bool{true, false} {
		r := geom.RectXYWH(2, 2, 40, 24)
		p1 := NewPainter()
		c1 := p1.BeginOffscreen(geom.Size{W: 44, H: 28}, 2)
		c1.Clear(black)
		c1.FillRRectGradient(r, 6, red, blue, horizontal)

		p2 := NewPainter()
		c2 := p2.BeginOffscreen(geom.Size{W: 44, H: 28}, 2)
		c2.Clear(black)
		FillGradient(c2, r, 6, axisGradient(r, red, blue, horizontal))

		a, b := p1.SurfaceRGBA(), p2.SurfaceRGBA()
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				t.Fatalf("horizontal=%v: FillRRectGradient and the equivalent FillGradient "+
					"differ at byte %d (%d vs %d)", horizontal, i, a.Pix[i], b.Pix[i])
			}
		}
	}
}

// A gradient's corners have to be as crisp as a solid rounded rect's, because
// a card with a gradient background sits next to cards without one.
func TestGradientRespectsTheRoundedCorner(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		p := NewPainter()
		var c Canvas = p.BeginOffscreen(geom.Size{W: 40, H: 40}, 1)
		c.Clear(RGB(0, 1, 0))
		if wrap {
			c = plainCanvas{c}
		}
		FillGradient(c, geom.RectXYWH(0, 0, 40, 40), 16, LinearGradient(
			geom.Pt{}, geom.Pt{X: 40}, EvenStops(red, blue)...))
		img := p.SurfaceRGBA()
		// Just outside the corner arc the background must survive.
		if r, g, b, _ := at8(img, 1, 1); !(r < 40 && g > 200 && b < 40) {
			t.Errorf("wrapped=%v: the rounded corner was filled: (%d,%d,%d)", wrap, r, g, b)
		}
		// And the middle must not be.
		if _, g, _, _ := at8(img, 20, 20); g > 200 {
			t.Errorf("wrapped=%v: the middle was not filled", wrap)
		}
	}
}

// Nothing to draw must draw nothing, quietly: these inputs arrive from theme
// data and from a layout pass that collapsed a box to zero.
func TestFillGradientIgnoresNothingToDo(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		p := NewPainter()
		var c Canvas = p.BeginOffscreen(geom.Size{W: 20, H: 20}, 1)
		c.Clear(RGB(0, 1, 0))
		if wrap {
			c = plainCanvas{c}
		}
		full := geom.RectXYWH(0, 0, 20, 20)
		FillGradient(c, full, 0, nil)
		FillGradient(c, full, 0, LinearGradient(geom.Pt{}, geom.Pt{X: 20}))
		FillGradient(c, geom.Rect{}, 0, LinearGradient(geom.Pt{}, geom.Pt{X: 20}, EvenStops(red, blue)...))
		if r, g, b, _ := at8(p.SurfaceRGBA(), 10, 10); !(r < 40 && g > 200 && b < 40) {
			t.Errorf("wrapped=%v: something was painted: (%d,%d,%d)", wrap, r, g, b)
		}
	}
}

// A gradient inside PushTransform has to turn with the content. The brush is
// sampled in raw pixmap space, so the transform has to be undone in the brush
// rather than assumed away — a bare device-scale factor would leave the ramp
// stubbornly axis-aligned while the shape rotated.
func TestGradientRotatesWithTheTransform(t *testing.T) {
	// A horizontal red→blue ramp, rotated a quarter turn, becomes vertical.
	p := NewPainter()
	c := p.BeginOffscreen(geom.Size{W: 40, H: 40}, 2)
	c.Clear(black)
	c.PushTransform(Transform{Rotation: math.Pi / 2, PivotX: 20, PivotY: 20})
	FillGradient(c, geom.RectXYWH(0, 0, 40, 40), 0,
		LinearGradient(geom.Pt{}, geom.Pt{X: 40}, EvenStops(red, blue)...))
	c.PopTransform()

	img := p.SurfaceRGBA()
	// After a clockwise quarter turn the ramp's start is at the top, so red
	// must dominate there and blue at the bottom. The losing channel is not
	// asserted to be near zero: in linear light it climbs quickly, which is
	// the whole point of interpolating there.
	topR, _, topB, _ := at8(img, 40, 8)
	botR, _, botB, _ := at8(img, 40, 72)
	if topR < 2*topB {
		t.Errorf("top of the rotated gradient is (%d,_,%d), want red dominant — the ramp "+
			"did not turn with the content", topR, topB)
	}
	if botB < 2*botR {
		t.Errorf("bottom of the rotated gradient is (%d,_,%d), want blue dominant", botR, botB)
	}
	// And the gradient must still run top-to-bottom rather than left-to-right.
	lR, _, _, _ := at8(img, 8, 40)
	rR, _, _, _ := at8(img, 72, 40)
	if d := int(lR) - int(rR); d > 8 || d < -8 {
		t.Errorf("across the rotated ramp red went %d→%d; it should be constant there", lR, rR)
	}
}
