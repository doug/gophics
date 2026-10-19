package paint

import (
	"math"
	"sort"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
)

// GradientStop is one colour on a gradient's ramp. Pos is the position along
// the ramp in [0, 1]; values outside that are clamped when the gradient is
// built.
//
// Two stops at the same Pos make a hard edge: everything below the position
// interpolates towards the first of them and everything at or above it starts
// from the second, which is how a two-tone band or a pie slice is drawn
// without a seam shape. Stops may be given in any order — they are sorted on
// construction, stably, so the order of equal positions is the order they were
// written in and the hard edge faces the way the caller wrote it.
type GradientStop struct {
	Pos   float32
	Color Color
}

// EvenStops spreads colours evenly over the whole ramp — the common case, and
// shorter than writing the positions out. One colour yields a constant ramp.
func EvenStops(colors ...Color) []GradientStop {
	if len(colors) == 0 {
		return nil
	}
	if len(colors) == 1 {
		return []GradientStop{{Pos: 0, Color: colors[0]}}
	}
	out := make([]GradientStop, len(colors))
	for i, c := range colors {
		out[i] = GradientStop{Pos: float32(i) / float32(len(colors)-1), Color: c}
	}
	return out
}

// gradientKind is which geometry maps a point to a ramp position.
type gradientKind uint8

const (
	gradLinear gradientKind = iota
	gradRadial
	gradSweep
)

// Gradient is a multi-stop colour ramp plus the geometry that maps a point to
// a position on it. Build one with LinearGradient, RadialGradient or
// SweepGradient and paint it with FillGradient. Coordinates are logical
// pixels, like everything else a Canvas is handed.
//
// A Gradient is immutable once built and safe to keep across frames; building
// one per frame is also fine (it is two small allocations), but reusing one is
// how a backend that caches a brush or a gradient texture gets to recognise
// it.
//
// # Colour interpolation
//
// Stops are interpolated in **premultiplied linear-light sRGB**, and both
// halves of that matter.
//
// *Linear-light*, because interpolating the sRGB-encoded numbers directly
// walks through a dead zone: the sRGB encoding is roughly a square root of
// light, so averaging the encodings of two complementary colours lands near
// 0.5 encoded, which is only about 21% of the light the two ends average to.
// Red to cyan goes through a muddy mid grey that way; in linear light it
// passes through a bright neutral, which is what the two colours actually sum
// to and what every compositor that gets this right produces.
//
// *Premultiplied*, because a stop's colour is meaningless where its alpha is
// zero. A ramp from opaque red to transparent interpolated on straight
// components drags red towards whatever the transparent stop happens to name —
// usually black — and the midpoint paints a dark half-transparent red, the
// grey halo every fade-to-nothing gradient used to have. Premultiplying first
// makes the same ramp stay red all the way down to nothing.
//
// Alpha itself stays linear: it is a coverage fraction, not a light level, and
// putting it through the sRGB transfer function would bend every fade.
type Gradient struct {
	kind  gradientKind
	stops []GradientStop // sorted by Pos, positions clamped to [0,1]

	// Linear: ramp runs from a to b. Radial: centred on a with radius rad.
	// Sweep: centred on a, from angle ang0 to ang1 (radians, clockwise on
	// screen because y grows downwards).
	a, b       geom.Pt
	rad        float32
	ang0, ang1 float32
}

// LinearGradient ramps along the line from one point to the other. Points on
// the far side of either end take that end's colour (the ramp pads rather than
// repeating).
//
//	paint.FillGradient(c, r, 8, paint.LinearGradient(
//		r.Min, geom.Pt{X: r.Min.X, Y: r.Max.Y},
//		paint.EvenStops(top, middle, bottom)...))
func LinearGradient(from, to geom.Pt, stops ...GradientStop) *Gradient {
	return &Gradient{kind: gradLinear, a: from, b: to, stops: normalizeStops(stops)}
}

// RadialGradient ramps outwards from center, reaching the last stop at
// radius. Everything further out takes the last stop's colour.
func RadialGradient(center geom.Pt, radius float32, stops ...GradientStop) *Gradient {
	return &Gradient{kind: gradRadial, a: center, rad: radius, stops: normalizeStops(stops)}
}

// SweepGradient ramps around center through the angles from start to end, in
// radians. Angle 0 points along +X and angles increase clockwise on screen,
// because y grows downwards — so the usual colour wheel is start 0, end 2π.
// A negative sweep (end below start) runs anticlockwise.
func SweepGradient(center geom.Pt, start, end float32, stops ...GradientStop) *Gradient {
	return &Gradient{kind: gradSweep, a: center, ang0: start, ang1: end, stops: normalizeStops(stops)}
}

// normalizeStops clamps positions into range and sorts stably. Stability is
// the point: equal positions are a hard edge, and which colour is on which
// side of it is decided by the order the caller wrote them, so a sort that
// swapped them would silently invert the edge.
func normalizeStops(in []GradientStop) []GradientStop {
	if len(in) == 0 {
		return nil
	}
	out := make([]GradientStop, len(in))
	copy(out, in)
	for i := range out {
		out[i].Pos = clamp01(out[i].Pos)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos < out[j].Pos })
	return out
}

// At returns the colour at ramp position t, clamped to the ends. It is the
// gradient's whole colour story, independent of geometry: a custom Canvas
// backend that wants to build a texture or a shader LUT samples this, and the
// band fallback below and the analytic brush both go through it so that no two
// paths can disagree about what a gradient looks like.
func (g *Gradient) At(t float32) Color {
	n := len(g.stops)
	switch {
	case n == 0:
		return Color{} // nothing to paint
	case n == 1:
		return g.stops[0].Color
	}
	if t <= g.stops[0].Pos {
		return g.stops[0].Color
	}
	if t >= g.stops[n-1].Pos {
		return g.stops[n-1].Color
	}
	// The first stop strictly past t ends the segment t falls in. Searching
	// for "strictly past" rather than "at or past" is what gives duplicate
	// positions their hard edge: at t exactly on a doubled position the
	// search steps over both copies, so the segment starts at the second one
	// and the colour jumps.
	j := sort.Search(n, func(i int) bool { return g.stops[i].Pos > t })
	lo, hi := g.stops[j-1], g.stops[j]
	span := hi.Pos - lo.Pos
	if span <= 0 {
		return hi.Color
	}
	return lerpPremulLinear(lo.Color, hi.Color, (t-lo.Pos)/span)
}

// Param returns the ramp position of a logical point — t for At. It is
// unclamped for linear gradients beyond their ends only in the sense that At
// clamps; radial and sweep already fold their domain.
func (g *Gradient) Param(p geom.Pt) float32 {
	switch g.kind {
	case gradRadial:
		if g.rad == 0 {
			return 0
		}
		dx, dy := float64(p.X-g.a.X), float64(p.Y-g.a.Y)
		return float32(math.Hypot(dx, dy)) / g.rad
	case gradSweep:
		sweep := g.ang1 - g.ang0
		if sweep == 0 {
			return 0
		}
		dx, dy := float64(p.X-g.a.X), float64(p.Y-g.a.Y)
		if dx == 0 && dy == 0 {
			return 0
		}
		rel := float32(math.Atan2(dy, dx)) - g.ang0
		// Fold into one turn in the direction of the sweep, so the ramp
		// starts at ang0 wherever atan2 happened to put its branch cut.
		const twoPi = 2 * math.Pi
		if sweep > 0 {
			for rel < 0 {
				rel += twoPi
			}
			for rel >= twoPi {
				rel -= twoPi
			}
		} else {
			for rel > 0 {
				rel -= twoPi
			}
			for rel <= -twoPi {
				rel += twoPi
			}
		}
		return rel / sweep
	default:
		dx, dy := g.b.X-g.a.X, g.b.Y-g.a.Y
		lenSq := dx*dx + dy*dy
		if lenSq == 0 {
			return 0
		}
		return ((p.X-g.a.X)*dx + (p.Y-g.a.Y)*dy) / lenSq
	}
}

// Extent is how far, in logical pixels, the ramp is stretched when painted
// into r — the length of a linear gradient's axis, a radial gradient's radius,
// or the arc a sweep gradient's outermost visible point travels. It is what
// decides how finely a backend has to sample: a ramp spread over 400 pixels
// needs more steps than the same ramp over 12.
func (g *Gradient) Extent(r geom.Rect) float32 {
	switch g.kind {
	case gradRadial:
		return absF(g.rad)
	case gradSweep:
		return absF(g.ang1-g.ang0) * cornerDist(g.a, r)
	default:
		return float32(math.Hypot(float64(g.b.X-g.a.X), float64(g.b.Y-g.a.Y)))
	}
}

// cornerDist is the distance from p to the furthest corner of r. It bounds
// every point of r in every direction from p, which is how far a gradient's
// bands have to reach from their own anchor to cover the shape being filled —
// true even when that anchor is nowhere near it.
func cornerDist(p geom.Pt, r geom.Rect) float32 {
	d := float32(0)
	for _, c := range [4]geom.Pt{
		r.Min, {X: r.Max.X, Y: r.Min.Y}, {X: r.Min.X, Y: r.Max.Y}, r.Max,
	} {
		if v := float32(math.Hypot(float64(c.X-p.X), float64(c.Y-p.Y))); v > d {
			d = v
		}
	}
	return d
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// GradientFiller is the opt-in Canvas capability for gradient fills. A Canvas
// that implements it is saying it can do better than the band approximation
// FillGradient falls back to — the painter canvas evaluates the ramp per pixel
// through gg's brush, which has no banding at all.
//
// It is a capability rather than a Canvas method for the same reason Measurer
// is not: Canvas is implemented outside this package (by the scene recorder,
// and by anyone writing a backend), so a method added to it is a breaking
// change, whereas an unimplemented capability costs a caller nothing because
// FillGradient paints the gradient either way.
//
// The scene recorder implements it too, and records one op holding the
// *Gradient rather than the bands. That is not it "doing better" in the sense
// above — a display list has no pixels — but deferring the decision: the
// recorder cannot know the device scale of the surface it will be replayed
// onto, so the band count it would choose is a guess, where the replay target
// either has a real brush or can size bands against its own scale.
type GradientFiller interface {
	// FillGradientRRect fills the rounded rect r with g.
	FillGradientRRect(r geom.Rect, radius float32, g *Gradient)
}

// FillGradient fills the rounded rect r with g on any Canvas. A radius of 0 is
// a plain rectangle.
//
// Where the canvas can fill a gradient itself it does; otherwise the ramp is
// approximated by bands of solid colour clipped to the rounded rect, as many
// as the colours need and the pixels can show (see gradientBandCount). Either
// way the colours come from Gradient.At, so the two routes agree about what
// the gradient looks like and differ only in how finely they resolve it.
func FillGradient(c Canvas, r geom.Rect, radius float32, g *Gradient) {
	if g == nil || len(g.stops) == 0 || r.IsEmpty() {
		return
	}
	if gf, ok := c.(GradientFiller); ok {
		gf.FillGradientRRect(r, radius, g)
		return
	}
	paintGradientBands(c, r, radius, g, assumedBandScale)
}

// assumedBandScale is the device pixel ratio the band fallback works to when it
// is painting into a Canvas that cannot fill a gradient itself — in practice
// the scene recorder, which chooses its bands now and replays them later onto
// a surface whose scale is not known here.
//
// The assumption is deliberately high rather than 1. Guessing 1 and landing on
// a 2× display halves the band count and makes visible the banding this code
// exists to avoid; guessing 2 and landing on a 1× display only draws some
// bands narrower than a pixel. One of those is a rendering bug and the other
// is a little wasted fill.
const assumedBandScale = 2

// deviceScale is this canvas's device pixel ratio — physical pixels per
// logical one.
func (c *ggCanvas) deviceScale() float32 {
	s := float32(c.dc.DeviceScale())
	if s <= 0 {
		return 1
	}
	return s
}

// FillGradientRRect implements GradientFiller.
//
// On the CPU rasterizer the ramp becomes a brush sampled per pixel, which is
// exact and has no banding. On the GPU the tile rasterizer understands only a
// single solid colour per shape, so the same bands FillGradient falls back to
// are composited instead — the long-standing arrangement for gradients here,
// now covering multi-stop, radial and sweep as well as the original two-stop
// axis ramp. (A native GPU gradient shader would remove the band overdraw;
// tracked.)
func (c *ggCanvas) FillGradientRRect(r geom.Rect, radius float32, g *Gradient) {
	if g == nil || len(g.stops) == 0 || r.IsEmpty() {
		return
	}
	if c.gpuActive() {
		paintGradientBands(c, r, radius, g, c.deviceScale())
		return
	}
	c.dc.SetFillBrush(c.gradientBrush(g))
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Fill()
	c.dc.SetColor(solidBrushReset) // back to a solid brush for the next shape
}

// solidBrushReset restores gg's paint to a solid brush after a gradient fill.
// Which colour it is does not matter — every draw call sets its own — but
// leaving the brush installed would gradient-fill the next shape.
var solidBrushReset = Color{A: 1}.nrgba()

// gradientBrush wraps g as a gg brush.
//
// gg samples a brush with *physical* pixel coordinates straight off the
// pixmap, with neither the device scale nor the current transform applied —
// while a Gradient's geometry, like every other coordinate a Canvas is handed,
// is in logical pixels. So the brush has to undo the full chain itself, and
// the inverse of (device scale × current transform) is exactly that.
//
// Getting this wrong is the documented 1×/2× trap in its purest form, and it
// was live here: the old two-stop path handed gg logical endpoints, so at
// scale 2 the ramp finished a quarter of the way across the shape and the rest
// was flat end colour. Inverting the matrix instead of multiplying by the
// scale also keeps a gradient correct inside PushTransform, where a bare
// scale factor would leave the ramp unrotated while the shape turned.
func (c *ggCanvas) gradientBrush(g *Gradient) gg.Brush {
	total := gg.Scale(c.dc.DeviceScale(), c.dc.DeviceScale()).Multiply(c.dc.GetTransform())
	inv := total.Invert()
	return gg.NewCustomBrush(func(x, y float64) gg.RGBA {
		p := inv.TransformPoint(gg.Pt(x, y))
		return g.At(g.Param(geom.Pt{X: float32(p.X), Y: float32(p.Y)})).ggRGBA()
	})
}

// FillRRectGradient implements Canvas. It is the original two-stop,
// axis-aligned gradient call, kept because the scene recorder records it as a
// single op; it now runs on the same machinery as FillGradient, so it picks up
// the linear-light interpolation and the HiDPI fix along with everything else.
func (c *ggCanvas) FillRRectGradient(r geom.Rect, radius float32, from, to Color, horizontal bool) {
	c.FillGradientRRect(r, radius, axisGradient(r, from, to, horizontal))
}

// axisGradient is the two-stop ramp across r that Canvas.FillRRectGradient
// describes: left to right, or top to bottom.
func axisGradient(r geom.Rect, from, to Color, horizontal bool) *Gradient {
	end := geom.Pt{X: r.Min.X, Y: r.Max.Y}
	if horizontal {
		end = geom.Pt{X: r.Max.X, Y: r.Min.Y}
	}
	return LinearGradient(r.Min, end, GradientStop{0, from}, GradientStop{1, to})
}

// maxGradientBands bounds the band fallback. 256 bands cover a full-range
// 8-bit ramp one level at a time, so nothing above it can improve a gradient —
// it can only cost fill, and on a recorder it also costs one display-list op
// per band, which the scene differ then has to walk every frame.
const maxGradientBands = 256

// minGradientBands keeps a band fill from degenerating into a flat one when
// the adaptive count rounds down on a tiny shape.
const minGradientBands = 2

// gradientBandCount is how many solid bands approximate g over r at the given
// device pixel ratio.
//
// Two limits meet here, and the smaller wins.
//
// The *ramp* limit is how many bands the colours need: one per 8-bit level the
// ramp actually travels through, because a band narrower than that cannot
// change the picture. See rampSteps. A subtle grey-to-grey wash costs a
// handful of bands where a saturated sweep costs hundreds — the fixed count
// this replaced spent the same 64 on both, which was wasteful on one and
// visibly striped on the other.
//
// The *extent* limit is how many bands the geometry can show: more bands than
// device pixels across the ramp cannot be distinguished, so a 10-pixel chip
// gets ten bands however violent its colours are. This is the limit that has
// to be counted in physical pixels — the same ramp over the same logical
// rectangle needs twice the bands at 2×, and computing it in logical pixels is
// how a gradient ends up smooth on the development machine and striped on the
// Retina one.
func gradientBandCount(g *Gradient, r geom.Rect, scale float32) int {
	if scale <= 0 {
		scale = 1
	}
	n := min(g.rampSteps(), int(math.Ceil(float64(g.Extent(r)*scale))))
	return min(max(n, minGradientBands), maxGradientBands)
}

// rampStepSamples is how finely rampSteps walks the ramp. It only has to be
// fine enough to catch the shape of the curve, not the 8-bit levels: the
// summed differences converge from below, so more samples would only ever ask
// for more bands than the cap allows anyway.
const rampStepSamples = 64

// rampSteps is how many 8-bit levels the ramp travels through from end to end
// — its arc length in colour, measured on the colours that actually come out.
//
// It is measured by sampling rather than computed from the stops, because the
// interpolation is not linear in the output. A ramp out of black spends a
// disproportionate share of its length in the first few percent, since the
// sRGB encoding's slope there is nearly thirteen to one; a stop list says
// nothing about that, and a count derived from the stop colours alone
// under-asks for bands by an order of magnitude exactly where banding shows
// most. Premultiplied fades bend the output the same way.
func (g *Gradient) rampSteps() int {
	if len(g.stops) < 2 {
		return 0
	}
	total := float32(0)
	prev := g.At(0)
	for i := 1; i <= rampStepSamples; i++ {
		cur := g.At(float32(i) / rampStepSamples)
		total += max(absF(cur.R-prev.R), absF(cur.G-prev.G),
			absF(cur.B-prev.B), absF(cur.A-prev.A))
		prev = cur
	}
	return int(math.Ceil(float64(total) * 255))
}

// paintGradientBands approximates g over the rounded rect r with bands of
// solid colour, using nothing but ordinary Canvas calls so that it works on
// any backend — the GPU rasterizer, which has no gradient brush, and the scene
// recorder, which records the bands as plain shapes and replays them anywhere.
//
// Each band reaches about a device pixel *past* its own end, and the next band
// is drawn over the overhang. Without it the anti-aliased edges of two
// neighbouring bands each cover part of the seam pixel and up to a quarter of
// the backdrop shows through as a hairline; with it, the next band's leading
// edge blends against its neighbour's colour instead, which is the smoothing
// one wants anyway. The overhang must point forwards and nothing may reach
// backwards: a band that also started early would win the pixels of the band
// before it, and the ramp would come out shifted by a whole band — which, when
// the bands are a pixel wide, is a twenty-level error.
//
// A *translucent* gradient still double-blends along each seam, because the
// overhang is painted over rather than replaced. That is the one thing this
// approximation gets visibly wrong, and it is why a backend with a real
// gradient brush (see GradientFiller) is used wherever one exists.
func paintGradientBands(c Canvas, r geom.Rect, radius float32, g *Gradient, scale float32) {
	n := gradientBandCount(g, r, scale)
	over := 1 / scale // logical pixels that come to about one device pixel

	c.PushClipRRect(r, radius)
	defer c.PopClip()

	switch g.kind {
	case gradRadial:
		radialBands(c, r, g, n)
	case gradSweep:
		sweepBands(c, r, g, n, over)
	default:
		linearBands(c, r, g, n, over)
	}
}

// bandColor is the colour of band i of n — sampled at the band's middle, so
// the approximation is centred on the true ramp rather than lagging half a
// band behind it.
func bandColor(g *Gradient, i, n int) Color {
	return g.At((float32(i) + 0.5) / float32(n))
}

// linearBands fills slabs perpendicular to the gradient's axis.
//
// An axis that is already horizontal or vertical — every gradient the old
// two-stop call makes, and most of the rest — is filled with plain rectangles
// and allocates nothing. An oblique axis needs rotated quads, which on a
// generic Canvas means one retained Path each; that is the price of an angle,
// and it is paid only by gradients that ask for one.
func linearBands(c Canvas, r geom.Rect, g *Gradient, n int, over float32) {
	dx, dy := g.b.X-g.a.X, g.b.Y-g.a.Y
	axis := float32(math.Hypot(float64(dx), float64(dy)))
	if axis == 0 {
		c.FillRect(r, g.At(0))
		return
	}
	// Reach far enough along and across the axis to cover the whole shape, so
	// the first and last bands pad the ramp out to the clip's edges. The
	// distance to the furthest corner bounds every point of r in every
	// direction, including the case the shape's own diagonal would miss: an
	// axis anchored well outside the rectangle it is filling, which is
	// ordinary when a gradient is defined once in a parent's coordinates and
	// used on several children.
	reach := cornerDist(g.a, r)
	ux, uy := dx/axis, dy/axis
	step := axis / float32(n)

	if dy == 0 || dx == 0 {
		vertical := dx == 0
		for i := range n {
			s0, s1 := float32(i)*step, float32(i+1)*step+over
			if i == 0 {
				s0 = -reach
			}
			if i == n-1 {
				s1 = axis + reach
			}
			var band geom.Rect
			if vertical {
				y0, y1 := g.a.Y+uy*s0, g.a.Y+uy*s1
				band = geom.Rect{Min: geom.Pt{X: r.Min.X, Y: min(y0, y1)}, Max: geom.Pt{X: r.Max.X, Y: max(y0, y1)}}
			} else {
				x0, x1 := g.a.X+ux*s0, g.a.X+ux*s1
				band = geom.Rect{Min: geom.Pt{X: min(x0, x1), Y: r.Min.Y}, Max: geom.Pt{X: max(x0, x1), Y: r.Max.Y}}
			}
			c.FillRect(band, bandColor(g, i, n))
		}
		return
	}

	vx, vy := -uy, ux // across the axis
	for i := range n {
		s0, s1 := float32(i)*step, float32(i+1)*step+over
		if i == 0 {
			s0 = -reach
		}
		if i == n-1 {
			s1 = axis + reach
		}
		p := NewPath()
		p.MoveTo(geom.Pt{X: g.a.X + ux*s0 + vx*reach, Y: g.a.Y + uy*s0 + vy*reach})
		p.LineTo(geom.Pt{X: g.a.X + ux*s1 + vx*reach, Y: g.a.Y + uy*s1 + vy*reach})
		p.LineTo(geom.Pt{X: g.a.X + ux*s1 - vx*reach, Y: g.a.Y + uy*s1 - vy*reach})
		p.LineTo(geom.Pt{X: g.a.X + ux*s0 - vx*reach, Y: g.a.Y + uy*s0 - vy*reach})
		c.FillPath(p.Close(), bandColor(g, i, n))
	}
}

// radialBands fills discs from the outside in.
//
// Discs rather than rings: a ring is a two-contour path, and the tile
// rasterizer this fallback exists for mishandles multi-contour winding — it
// fills the hole solid, the same defect that forces glyph runs onto the
// analytic filler. Discs are one contour each and always correct in shape, at
// the cost of overdraw, and of a translucent radial ramp compositing
// approximately because the inner discs lie over the outer ones instead of
// beside them. An opaque gradient, which is nearly all of them, is exact.
//
// Nesting discs is also why no overhang is needed here: every disc is drawn
// over the one outside it, so each boundary already has a neighbour's colour
// underneath it and no seam can show the backdrop.
func radialBands(c Canvas, r geom.Rect, g *Gradient, n int) {
	// Beyond the last stop the ramp pads, and the shape may well be larger
	// than the gradient's circle, so lay down the far colour first.
	c.FillRect(r, g.At(1))
	step := g.rad / float32(n)
	for i := n - 1; i >= 0; i-- {
		c.FillPath(circlePath(g.a, float32(i+1)*step), bandColor(g, i, n))
	}
}

// sweepBands fills wedges around the centre, each reaching past its own
// trailing edge so that the next one's leading edge has a neighbour's colour
// underneath it rather than the backdrop.
//
// The overhang is a whole band's *angle*, not a pixel width, and that is the
// difference between this working and not. A wedge's edges converge on the
// centre, so an overhang specified as a fixed arc length at the rim shrinks to
// nothing further in — and where it drops below the width of an anti-aliased
// edge, the two neighbours' partial coverages no longer sum to one and a
// radial hairline of the backdrop shows through the whole fan. An angular
// overhang keeps the same proportion at every radius. Doubling each wedge is
// twice the fill, which on the approximate path is the right trade.
func sweepBands(c Canvas, r geom.Rect, g *Gradient, n int, over float32) {
	reach := cornerDist(g.a, r)
	if reach <= 0 {
		return
	}
	sweep := g.ang1 - g.ang0
	step := sweep / float32(n)
	// Still at least one device pixel at the rim, for the rare gradient whose
	// bands are finer than that.
	grow := step
	if slop := over / reach; absF(slop) > absF(grow) {
		grow = slop
		if sweep < 0 {
			grow = -slop
		}
	}

	// Every wedge's edges meet at the centre, so the pixel there is covered by
	// two hundred-odd slivers of a few percent each and sequential
	// compositing never quite reaches opaque — the backdrop shows as a dot in
	// the middle of the fan. A flat base under the whole shape costs one fill
	// and makes that impossible. The start colour is the conventional answer
	// at a sweep's centre, where the angle is undefined.
	c.FillRect(r, g.At(0))

	// A sweep short of a full turn leaves the rest of the circle to the two
	// end colours, as At pads them. They go down first so the ramp's own
	// wedges blend over them rather than over the backdrop.
	if absF(sweep) < 2*math.Pi-1e-4 {
		gapStart, gapEnd := g.ang1, g.ang0+2*math.Pi
		if sweep < 0 {
			gapStart, gapEnd = g.ang1+2*math.Pi, g.ang0
		}
		mid := (gapStart + gapEnd) / 2
		c.FillPath(wedgePath(g.a, reach, gapStart-absF(grow), mid+absF(grow)), g.At(1))
		c.FillPath(wedgePath(g.a, reach, mid-absF(grow), gapEnd+absF(grow)), g.At(0))
	}

	for i := range n {
		a0 := g.ang0 + float32(i)*step
		a1 := a0 + step + grow
		if i == n-1 {
			// The last wedge stops exactly on the sweep's end rather than
			// overhanging. On a full turn that end *is* the first wedge's
			// start, which was drawn first, so the seam already has a colour
			// under it — and an overhang there would put the end of the ramp
			// on top of its beginning, which is visible as a wrong sliver at
			// whatever angle the sweep happens to start at.
			a1 = g.ang1
		}
		c.FillPath(wedgePath(g.a, reach, a0, a1), bandColor(g, i, n))
	}
}

// kappa is the cubic Bézier control-point fraction that draws a quarter circle
// to within about one part in 4000 of the radius — far under a pixel at any
// size a UI gradient is painted at.
const kappa = 0.5522847498307933

// circlePath is a disc of the given radius as four cubic quarters.
func circlePath(center geom.Pt, rad float32) *Path {
	k := rad * kappa
	cx, cy := center.X, center.Y
	p := NewPath()
	p.MoveTo(geom.Pt{X: cx + rad, Y: cy})
	p.CubeTo(geom.Pt{X: cx + rad, Y: cy + k}, geom.Pt{X: cx + k, Y: cy + rad}, geom.Pt{X: cx, Y: cy + rad})
	p.CubeTo(geom.Pt{X: cx - k, Y: cy + rad}, geom.Pt{X: cx - rad, Y: cy + k}, geom.Pt{X: cx - rad, Y: cy})
	p.CubeTo(geom.Pt{X: cx - rad, Y: cy - k}, geom.Pt{X: cx - k, Y: cy - rad}, geom.Pt{X: cx, Y: cy - rad})
	p.CubeTo(geom.Pt{X: cx + k, Y: cy - rad}, geom.Pt{X: cx + rad, Y: cy - k}, geom.Pt{X: cx + rad, Y: cy})
	return p.Close()
}

// wedgePath is a pie slice from a0 to a1 at the given radius, its arc
// subdivided finely enough that the chord error stays well under a pixel even
// when a sweep gradient is coarse enough for one wedge to span a quadrant.
func wedgePath(center geom.Pt, rad, a0, a1 float32) *Path {
	const maxArc = math.Pi / 36 // 5°
	steps := int(math.Ceil(float64(absF(a1-a0)) / maxArc))
	if steps < 1 {
		steps = 1
	}
	p := NewPath()
	p.MoveTo(center)
	for i := 0; i <= steps; i++ {
		a := float64(a0 + (a1-a0)*float32(i)/float32(steps))
		p.LineTo(geom.Pt{
			X: center.X + rad*float32(math.Cos(a)),
			Y: center.Y + rad*float32(math.Sin(a)),
		})
	}
	return p.Close()
}

// lerpPremulLinear interpolates two straight-alpha sRGB colours in
// premultiplied linear light — see Gradient's doc comment for why that is the
// space, and gradient_test.go for the two failures it is there to prevent.
func lerpPremulLinear(a, b Color, t float32) Color {
	// Short-circuit the ends. Not only for speed: the sRGB round trip is
	// accurate to about one part in ten thousand, and without this a stop's
	// own colour would come back a hair off the colour the caller wrote,
	// which turns an exact golden assertion into an approximate one for no
	// reason.
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	ar, ag, ab := srgbToLinear(a.R)*a.A, srgbToLinear(a.G)*a.A, srgbToLinear(a.B)*a.A
	br, bg, bb := srgbToLinear(b.R)*b.A, srgbToLinear(b.G)*b.A, srgbToLinear(b.B)*b.A
	r := ar + (br-ar)*t
	gc := ag + (bg-ag)*t
	bl := ab + (bb-ab)*t
	al := a.A + (b.A-a.A)*t
	if al <= 0 {
		// No light and no coverage: the hue is genuinely undefined here, and
		// returning the un-premultiplied 0/0 would be a NaN.
		return Color{}
	}
	return Color{
		R: linearToSRGB(r / al),
		G: linearToSRGB(gc / al),
		B: linearToSRGB(bl / al),
		A: al,
	}
}

// srgbToLinear and linearToSRGB are the IEC 61966-2-1 transfer function and
// its inverse. The linear segment near black is not decoration: a pure
// power-2.2 approximation has infinite slope at 0, which makes a ramp out of
// black quantise into a visible step.
func srgbToLinear(v float32) float32 {
	v = clamp01(v)
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow(float64((v+0.055)/1.055), 2.4))
}

func linearToSRGB(v float32) float32 {
	v = clamp01(v)
	if v <= 0.0031308 {
		return v * 12.92
	}
	return float32(1.055*math.Pow(float64(v), 1/2.4) - 0.055)
}
