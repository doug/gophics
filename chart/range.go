package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Span is one Lo–Hi interval at position X (categorical when Label is set).
type Span struct {
	X, Lo, Hi float64
	Label     string
	Color     paint.Color
}

// RangeMark draws a floating bar from Lo to Hi at each X — for min/max ranges,
// cash-flow in/out, or error bars.
type RangeMark struct {
	Data  []Span
	Name  string
	Color paint.Color
	Width float32 // 0..1 fraction of the slot; 0 → 0.4
}

func (m RangeMark) xDomain() (lo, hi float64, c []string) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1), nil
	}
	if c = spanCats(m.Data); c != nil {
		return -0.5, float64(len(c)) - 0.5, c
	}
	lo, hi = m.Data[0].X, m.Data[0].X
	for _, s := range m.Data {
		lo, hi = min(lo, s.X), max(hi, s.X)
	}
	return lo, hi, nil
}

func (m RangeMark) yDomain() (lo, hi float64) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1)
	}
	lo, hi = m.Data[0].Lo, m.Data[0].Hi
	for _, s := range m.Data {
		lo, hi = min(lo, s.Lo), max(hi, s.Hi)
	}
	return lo, hi
}

func (m RangeMark) draw(p Plot) {
	if len(m.Data) == 0 {
		return
	}
	col := colorOr(m.Color, p.th.series[p.series%len(p.th.series)])
	frac := m.Width
	if frac <= 0 {
		frac = 0.4
	}
	xs := make([]float64, len(m.Data))
	for i, s := range m.Data {
		xs[i] = s.X
	}
	bw := seriesSlot(p, xs) * frac
	for _, s := range m.Data {
		cx := p.px(s.X)
		top, bot := p.py(s.Hi), p.py(s.Lo)
		cy := (top + bot) / 2
		half := (bot - top) / 2 * p.T // grow from the center
		r := geom.RectXYWH(cx-bw/2, cy-half, bw, half*2)
		p.Canvas.FillRRect(r, min(bw*0.3, 4), colorOr(s.Color, col))
	}
}

func (m RangeMark) markName() string { return m.Name }

func spanCats(d []Span) []string {
	out := make([]string, len(d))
	for i, s := range d {
		if s.Label == "" {
			return nil
		}
		out[i] = s.Label
	}
	return out
}
