package chart

import (
	"math"

	"github.com/doug/gossamer/paint"
)

// PointMark draws a filled dot per datum — a scatter plot, or the vertices of a
// line series.
type PointMark struct {
	Data  []Datum
	Color paint.Color // zero → series color
	Size  float32     // diameter in px; 0 → 7
}

func (m PointMark) xDomain() (lo, hi float64, c []string) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1), nil
	}
	if c = cats(m.Data); c != nil {
		return -0.5, float64(len(c)) - 0.5, c
	}
	lo, hi = minMaxX(m.Data)
	return lo, hi, nil
}

func (m PointMark) yDomain() (lo, hi float64) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1)
	}
	return minMaxY(m.Data)
}

func (m PointMark) draw(p Plot) {
	col := colorOr(m.Color, p.th.series[p.series%len(p.th.series)])
	d := m.Size
	if d <= 0 {
		d = 7
	}
	d *= 0.4 + 0.6*p.T // subtle grow-in
	for _, pt := range m.Data {
		dot(p.Canvas, p.px(pt.X), p.py(pt.Y), d, colorOr(pt.Color, col))
	}
}

func (m PointMark) seriesData() []Datum    { return m.Data }
func (m PointMark) baseColor() paint.Color { return m.Color }
