package chart

import (
	"math"

	"github.com/doug/gophics/paint"
)

// PointMark draws a filled dot per datum — a scatter plot, or the vertices of a
// line series.
type PointMark struct {
	Data  []Datum
	Name  string      // legend label (optional)
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

func (m PointMark) draw(p plot) {
	col := colorOr(m.Color, p.th.series[p.series%len(p.th.series)])
	d := m.Size
	if d <= 0 {
		d = 7
	}
	d *= 0.4 + 0.6*p.T // subtle grow-in
	p.Canvas.DrawMarks(dotBatch(p, m.Data, d, col))
}

func (m PointMark) seriesData() []Datum    { return m.Data }
func (m PointMark) baseColor() paint.Color { return m.Color }

func (m PointMark) markName() string { return m.Name }

// dotBatch builds one Marks batch for data, so a scatter is a single draw
// rather than a fill per point.
//
// That distinction is not an optimisation at this scale, it is whether the
// chart is interactive: as separate fills, 10,000 points cost ~65ms a frame,
// nearly all of it in the rasterizer deciding the same circle over and over.
// A per-datum Color still shares the batch — colour is per mark, and only the
// shape and size have to match.
func dotBatch(p plot, data []Datum, d float32, col paint.Color) *paint.Marks {
	b := &paint.Marks{
		Kind:  paint.MarkCircle,
		X:     make([]float32, len(data)),
		Y:     make([]float32, len(data)),
		Size:  []float32{d},
		Color: make([]paint.Color, len(data)),
	}
	for i, pt := range data {
		b.X[i], b.Y[i] = p.px(pt.X), p.py(pt.Y)
		b.Color[i] = colorOr(pt.Color, col)
	}
	return b
}
