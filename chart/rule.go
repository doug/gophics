package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// RuleMark draws a single reference line across the plot — a target, threshold,
// or average. Horizontal draws at Y=Value; otherwise vertical at X=Value.
type RuleMark struct {
	Value      float64
	Horizontal bool
	Color      paint.Color
	Width      float32
	Dash       float32 // dash length in px; 0 → solid
}

func (r RuleMark) xDomain() (lo, hi float64, c []string) {
	if r.Horizontal {
		return math.Inf(1), math.Inf(-1), nil
	}
	return r.Value, r.Value, nil
}

func (r RuleMark) yDomain() (lo, hi float64) {
	if r.Horizontal {
		return r.Value, r.Value
	}
	return math.Inf(1), math.Inf(-1)
}

func (r RuleMark) draw(p plot) {
	col := colorOr(r.Color, gray(0.5, 0.7))
	w := r.Width
	if w <= 0 {
		w = 1.5
	}
	var a, b geom.Pt
	if r.Horizontal {
		y := p.py(r.Value)
		a, b = geom.Pt{X: p.Area.Min.X, Y: y}, geom.Pt{X: p.Area.Max.X, Y: y}
	} else {
		x := p.px(r.Value)
		a, b = geom.Pt{X: x, Y: p.Area.Min.Y}, geom.Pt{X: x, Y: p.Area.Max.Y}
	}
	if r.Dash <= 0 {
		p.Canvas.Line(a, b, w, col)
		return
	}
	dashLine(p.Canvas, a, b, w, r.Dash, col)
}

// dashLine strokes a dashed segment with equal on/off runs of length dash.
func dashLine(c paint.Canvas, a, b geom.Pt, w, dash float32, col paint.Color) {
	d := b.Sub(a)
	length := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if length == 0 {
		return
	}
	ux, uy := d.X/length, d.Y/length
	for t := float32(0); t < length; t += dash * 2 {
		e := t + dash
		if e > length {
			e = length
		}
		p0 := geom.Pt{X: a.X + ux*t, Y: a.Y + uy*t}
		p1 := geom.Pt{X: a.X + ux*e, Y: a.Y + uy*e}
		c.Line(p0, p1, w, col)
	}
}
