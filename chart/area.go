package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// AreaMark fills the region between a line series and the zero baseline. It is
// drawn as thin vertical columns sampled along the line, so it needs no path
// primitive; set Line to also stroke the top edge.
type AreaMark struct {
	Data  []Datum
	Name  string      // legend label (optional)
	Color paint.Color // zero → series color
	Alpha float32     // fill opacity; 0 → 0.16
	Line  float32     // top-edge stroke width in px; 0 → none
}

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

func (a AreaMark) draw(p Plot) {
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

	const step = float32(2)
	x0, xN := p.px(a.Data[0].X), p.px(a.Data[len(a.Data)-1].X)
	seg := 0
	for x := x0; x <= xN; x += step {
		for seg < len(a.Data)-2 && p.px(a.Data[seg+1].X) < x {
			seg++
		}
		ax, ay := p.px(a.Data[seg].X), p.py(a.Data[seg].Y)
		bx, by := p.px(a.Data[seg+1].X), p.py(a.Data[seg+1].Y)
		t := float32(0)
		if bx > ax {
			t = (x - ax) / (bx - ax)
		}
		y := ay + (by-ay)*t
		top, bot := y, y0
		if top > bot {
			top, bot = bot, top
		}
		p.Canvas.FillRect(geom.RectXYWH(x, top, step+0.6, bot-top), fill)
	}

	if a.Line > 0 {
		for i := 1; i < len(a.Data); i++ {
			p.Canvas.Line(
				geom.Pt{X: p.px(a.Data[i-1].X), Y: p.py(a.Data[i-1].Y)},
				geom.Pt{X: p.px(a.Data[i].X), Y: p.py(a.Data[i].Y)}, a.Line, base)
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
