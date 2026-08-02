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
	Smooth bool        // draw a Catmull-Rom curve through the points
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
	pts := make([]geom.Pt, len(l.Data))
	for i, d := range l.Data {
		pts[i] = geom.Pt{X: p.px(d.X), Y: p.py(d.Y)}
	}
	if l.Smooth && len(pts) >= 3 {
		p.Canvas.StrokePath(smoothPath(pts), w, col)
	} else {
		for i := 1; i < len(pts); i++ {
			p.Canvas.Line(pts[i-1], pts[i], w, col)
		}
	}
	if l.Points {
		for _, d := range l.Data {
			dot(p.Canvas, p.px(d.X), p.py(d.Y), w+3, col)
		}
	}
}

// smoothPath samples a Catmull-Rom spline through pts into a stroke path, so a
// LineMark can render a smooth curve without a native spline primitive.
func smoothPath(pts []geom.Pt) *paint.Path {
	const seg = 16
	p := paint.NewPath()
	p.MoveTo(pts[0])
	for i := 0; i < len(pts)-1; i++ {
		p0, p1, p2, p3 := pts[max(0, i-1)], pts[i], pts[i+1], pts[min(len(pts)-1, i+2)]
		for s := 1; s <= seg; s++ {
			p.LineTo(catmullRom(p0, p1, p2, p3, float32(s)/seg))
		}
	}
	return p
}

// catmullRom evaluates the centripetal-family Catmull-Rom spline at t in [0,1].
func catmullRom(p0, p1, p2, p3 geom.Pt, t float32) geom.Pt {
	t2, t3 := t*t, t*t*t
	f := func(a, b, c, d float32) float32 {
		return 0.5 * (2*b + (c-a)*t + (2*a-5*b+4*c-d)*t2 + (3*b-3*c+d-a)*t3)
	}
	return geom.Pt{X: f(p0.X, p1.X, p2.X, p3.X), Y: f(p0.Y, p1.Y, p2.Y, p3.Y)}
}

// dot fills a circle of diameter d centered at (x, y).
func dot(c paint.Canvas, x, y, d float32, col paint.Color) {
	r := d / 2
	c.FillRRect(geom.RectXYWH(x-r, y-r, d, d), r, col)
}

func (l LineMark) seriesData() []Datum    { return l.Data }
func (l LineMark) baseColor() paint.Color { return l.Color }

func (l LineMark) markName() string { return l.Name }
