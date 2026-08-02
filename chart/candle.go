package chart

import (
	"math"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Candle is one OHLC bar at position X.
type Candle struct {
	X, Open, High, Low, Close float64
	Label                     string
}

// CandleMark draws a candlestick series: a high–low wick and an open–close body,
// colored Up when Close ≥ Open, else Down.
type CandleMark struct {
	Data     []Candle
	Up, Down paint.Color
	Width    float32 // 0..1 fraction of the slot; 0 → 0.6
}

func (m CandleMark) xDomain() (lo, hi float64, c []string) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1), nil
	}
	lo, hi = m.Data[0].X, m.Data[0].X
	for _, c := range m.Data {
		lo, hi = min(lo, c.X), max(hi, c.X)
	}
	return lo - 0.5, hi + 0.5, nil
}

func (m CandleMark) yDomain() (lo, hi float64) {
	if len(m.Data) == 0 {
		return math.Inf(1), math.Inf(-1)
	}
	lo, hi = m.Data[0].Low, m.Data[0].High
	for _, c := range m.Data {
		lo, hi = min(lo, c.Low), max(hi, c.High)
	}
	return lo, hi
}

func (m CandleMark) draw(p Plot) {
	if len(m.Data) == 0 {
		return
	}
	up := colorOr(m.Up, paint.RGB(0.16, 0.63, 0.40))
	down := colorOr(m.Down, paint.RGB(0.86, 0.26, 0.28))
	frac := m.Width
	if frac <= 0 {
		frac = 0.6
	}
	xs := make([]float64, len(m.Data))
	for i, c := range m.Data {
		xs[i] = c.X
	}
	bw := seriesSlot(p, xs) * frac
	if p.T < 1 { // reveal left-to-right
		p.Canvas.PushClip(geom.RectXYWH(p.Area.Min.X, p.Area.Min.Y, p.Area.Dx()*p.T, p.Area.Dy()))
		defer p.Canvas.PopClip()
	}
	wick := max(float32(1), bw*0.12)
	for _, c := range m.Data {
		col := up
		if c.Close < c.Open {
			col = down
		}
		x := p.px(c.X)
		p.Canvas.Line(geom.Pt{X: x, Y: p.py(c.High)}, geom.Pt{X: x, Y: p.py(c.Low)}, wick, col)
		o, cl := p.py(c.Open), p.py(c.Close)
		top, bot := min(o, cl), max(o, cl)
		p.Canvas.FillRRect(geom.RectXYWH(x-bw/2, top, bw, max(1, bot-top)), min(bw*0.15, 2), col)
	}
}
