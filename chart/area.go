package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// AreaMark fills the region between a line series and the zero baseline as a
// single filled polygon; set Line to also stroke the top edge.
type AreaMark struct {
	Data  []Datum
	Name  string      // legend label (optional)
	Color paint.Color // zero → series color
	Alpha float32     // fill opacity; 0 → 0.16
	Line  float32     // top-edge stroke width in px; 0 → none
	// Stack groups this mark with others carrying the same name: each rests on
	// the sum of those before it instead of overlaying from zero. Empty (the
	// default) fills from the baseline, overlapping whatever is behind it.
	Stack string
}

func (a AreaMark) stackID() string    { return a.Stack }
func (a AreaMark) stackData() []Datum { return a.Data }

func (a AreaMark) xDomain() (lo, hi float64, c []string) {
	if len(a.Data) == 0 {
		return math.Inf(1), math.Inf(-1), nil
	}
	lo, hi = minMaxX(a.Data)
	return lo, hi, nil
}

func (a AreaMark) yDomain() (lo, hi float64) {
	if len(a.Data) == 0 {
		return math.Inf(1), math.Inf(-1)
	}
	lo, hi = minMaxY(a.Data)
	return min(lo, 0), max(hi, 0) // area anchors at the baseline
}

func (a AreaMark) draw(p plot) {
	if len(a.Data) < 2 {
		return
	}
	base := colorOr(a.Color, p.th.series[p.series%len(p.th.series)])
	fill := base
	if fill.A = a.Alpha; fill.A <= 0 {
		fill.A = 0.16
	}
	y0 := clamp(p.py(0), p.Area.Min.Y, p.Area.Max.Y)

	if p.T < 1 { // reveal left-to-right, matching LineMark
		p.Canvas.PushClip(geom.RectXYWH(p.Area.Min.X, p.Area.Min.Y, p.Area.Dx()*p.T, p.Area.Dy()))
		defer p.Canvas.PopClip()
	}

	// The area is one polygon — forward along the series, back along the
	// baseline — filled in a single pass. It used to be sampled into a run of
	// overlapping translucent columns, which can't composite cleanly: every
	// shared edge is either double-covered (a dark seam) or partly covered (a
	// light one), and a wide chart turned into a picket fence. A series that
	// crosses the baseline still fills correctly, because FillPath uses
	// non-zero winding and the sub-loop below the baseline simply winds the
	// other way.
	// Stacked, the lower edge is not the flat baseline but the running total
	// of the marks below, so it is walked back point by point rather than
	// closed with one straight line. topAt/botAt collapse the two cases.
	topAt := func(d Datum) float32 { return p.py(stackBaseAt(p, d.X) + d.Y) }
	botAt := func(d Datum) float32 {
		if p.base == nil {
			return y0
		}
		return clamp(p.py(stackBaseAt(p, d.X)), p.Area.Min.Y, p.Area.Max.Y)
	}

	poly := paint.NewPath()
	poly.MoveTo(geom.Pt{X: p.px(a.Data[0].X), Y: botAt(a.Data[0])})
	for _, d := range a.Data {
		poly.LineTo(geom.Pt{X: p.px(d.X), Y: topAt(d)})
	}
	for i := len(a.Data) - 1; i >= 0; i-- {
		poly.LineTo(geom.Pt{X: p.px(a.Data[i].X), Y: botAt(a.Data[i])})
	}
	p.Canvas.FillPath(poly.Close(), fill)

	if a.Line > 0 {
		for i := 1; i < len(a.Data); i++ {
			p.Canvas.Line(
				geom.Pt{X: p.px(a.Data[i-1].X), Y: topAt(a.Data[i-1])},
				geom.Pt{X: p.px(a.Data[i].X), Y: topAt(a.Data[i])}, a.Line, base)
		}
	}
}

func (a AreaMark) seriesData() []Datum    { return a.Data }
func (a AreaMark) baseColor() paint.Color { return a.Color }

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (a AreaMark) markName() string { return a.Name }
