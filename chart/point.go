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
	// One batch rather than a fill per point. A scatter is the case where that
	// distinction stops being an optimisation and becomes whether the chart is
	// interactive at all: as separate fills, 10,000 points cost ~65ms a frame,
	// nearly all of it in the rasterizer deciding the same circle over and
	// over.
	batch := &paint.Marks{
		Kind:  paint.MarkCircle,
		X:     make([]float32, len(m.Data)),
		Y:     make([]float32, len(m.Data)),
		Size:  []float32{d},
		Color: make([]paint.Color, len(m.Data)),
	}
	for i, pt := range m.Data {
		batch.X[i], batch.Y[i] = p.px(pt.X), p.py(pt.Y)
		batch.Color[i] = colorOr(pt.Color, col)
	}
	p.Canvas.DrawMarks(batch)
}

func (m PointMark) seriesData() []Datum    { return m.Data }
func (m PointMark) baseColor() paint.Color { return m.Color }

func (m PointMark) markName() string { return m.Name }
