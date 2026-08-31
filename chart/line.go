package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
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

func (l LineMark) draw(p plot) {
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

// smoothPath samples a monotone cubic through pts into a stroke path, so a
// LineMark can render a smooth curve without a native spline primitive.
//
// Monotone rather than Catmull-Rom, which this used to be. Catmull-Rom
// overshoots: on a flat-spike-flat series (what a live chart looks like when a
// new reading arrives) it bulged 5.9 units past a data maximum of 100, so the
// curve showed a value that is not in the data and then settled back as the
// next point landed. On a chart that is not a cosmetic wobble, it is the plot
// asserting something false, and it is the "jarring" part of a line that
// updates.
//
// Fritsch-Carlson tangents cannot overshoot: the curve stays inside the range
// of the points bracketing each interval, and a run of equal values draws
// flat instead of rippling. Where a series is not strictly increasing in X
// (unsorted or duplicated), the method is undefined and it falls back to
// straight segments rather than inventing a shape.
func smoothPath(pts []geom.Pt) *paint.Path {
	p := paint.NewPath()
	p.MoveTo(pts[0])
	m, ok := monotoneTangents(pts)
	if !ok {
		for _, q := range pts[1:] {
			p.LineTo(q)
		}
		return p
	}
	const seg = 16
	for i := 0; i < len(pts)-1; i++ {
		p0, p1 := pts[i], pts[i+1]
		h := p1.X - p0.X
		for s := 1; s <= seg; s++ {
			t := float32(s) / seg
			t2, t3 := t*t, t*t*t
			// Hermite basis.
			h00 := 2*t3 - 3*t2 + 1
			h10 := t3 - 2*t2 + t
			h01 := -2*t3 + 3*t2
			h11 := t3 - t2
			p.LineTo(geom.Pt{
				X: p0.X + h*t,
				Y: h00*p0.Y + h10*h*m[i] + h01*p1.Y + h11*h*m[i+1],
			})
		}
	}
	return p
}

// monotoneTangents returns Fritsch-Carlson tangents for pts, or ok=false when
// X is not strictly increasing, which the method requires.
func monotoneTangents(pts []geom.Pt) ([]float32, bool) {
	n := len(pts)
	d := make([]float32, n-1) // secant slopes
	for i := 0; i < n-1; i++ {
		dx := pts[i+1].X - pts[i].X
		if dx <= 0 {
			return nil, false
		}
		d[i] = (pts[i+1].Y - pts[i].Y) / dx
	}

	m := make([]float32, n)
	m[0], m[n-1] = d[0], d[n-2]
	for i := 1; i < n-1; i++ {
		// A local extremum gets a flat tangent; without this the curve would
		// swing past the peak it is supposed to touch.
		if d[i-1]*d[i] <= 0 {
			m[i] = 0
			continue
		}
		m[i] = (d[i-1] + d[i]) / 2
	}

	// Clamp each tangent into the Fritsch-Carlson circle of radius 3. This is
	// the step that makes overshoot impossible rather than merely unlikely.
	for i := 0; i < n-1; i++ {
		if d[i] == 0 {
			m[i], m[i+1] = 0, 0
			continue
		}
		a, b := m[i]/d[i], m[i+1]/d[i]
		if s := a*a + b*b; s > 9 {
			t := 3 / float32(math.Sqrt(float64(s)))
			m[i], m[i+1] = t*a*d[i], t*b*d[i]
		}
	}
	return m, true
}

// dot fills a circle of diameter d centered at (x, y).
func dot(c paint.Canvas, x, y, d float32, col paint.Color) {
	r := d / 2
	c.FillRRect(geom.RectXYWH(x-r, y-r, d, d), r, col)
}

func (l LineMark) seriesData() []Datum    { return l.Data }
func (l LineMark) baseColor() paint.Color { return l.Color }

func (l LineMark) markName() string { return l.Name }
