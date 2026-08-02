package chart

import (
	"math"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// LineMark connects its data points in order with straight segments. On mount it
// draws on left-to-right (via a reveal clip) when the chart animates.
type LineMark struct {
	Data   []Datum
	Name   string      // legend label (optional)
	Color  paint.Color // zero → series color
	Width  float32     // px; 0 → 2
	Points bool        // also draw a dot at each vertex
}

func (l LineMark) xDomain() (lo, hi float64, c []string) {
	if len(l.Data) == 0 {
		return math.Inf(1), math.Inf(-1), nil
	}
	lo, hi = minMaxX(l.Data)
	return lo, hi, nil
}

func (l LineMark) yDomain() (lo, hi float64) {
	if len(l.Data) == 0 {
		return math.Inf(1), math.Inf(-1)
	}
	return minMaxY(l.Data)
}

func (l LineMark) draw(p Plot) {
	if len(l.Data) == 0 {
		return
	}
	col := colorOr(l.Color, p.th.series[p.series%len(p.th.series)])
	w := l.Width
	if w <= 0 {
		w = 2
	}
	if p.T < 1 { // reveal left-to-right
		p.Canvas.PushClip(geom.RectXYWH(p.Area.Min.X, p.Area.Min.Y, p.Area.Dx()*p.T, p.Area.Dy()))
		defer p.Canvas.PopClip()
	}
	for i := 1; i < len(l.Data); i++ {
		a := geom.Pt{X: p.px(l.Data[i-1].X), Y: p.py(l.Data[i-1].Y)}
		b := geom.Pt{X: p.px(l.Data[i].X), Y: p.py(l.Data[i].Y)}
		p.Canvas.Line(a, b, w, col)
	}
	if l.Points {
		for _, d := range l.Data {
			dot(p.Canvas, p.px(d.X), p.py(d.Y), w+3, col)
		}
	}
}

// dot fills a circle of diameter d centered at (x, y).
func dot(c paint.Canvas, x, y, d float32, col paint.Color) {
	r := d / 2
	c.FillRRect(geom.RectXYWH(x-r, y-r, d, d), r, col)
}

func (l LineMark) seriesData() []Datum    { return l.Data }
func (l LineMark) baseColor() paint.Color { return l.Color }

func (l LineMark) markName() string { return l.Name }
