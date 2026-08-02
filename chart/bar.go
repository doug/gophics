package chart

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// BarMark draws a vertical bar per datum, from the zero baseline to Y. With
// categorical data (see Values) bars sit in their band slots; with numeric X
// they're centered on X and sized to the smallest gap.
type BarMark struct {
	Data   []Datum
	Color  paint.Color // zero → series color
	Corner float32     // top corner radius in px; 0 → a small default
	Width  float32     // 0..1 fraction of the slot; 0 → 0.7
}

func (b BarMark) xDomain() (lo, hi float64, c []string) {
	if c = cats(b.Data); c != nil {
		return -0.5, float64(len(c)) - 0.5, c
	}
	lo, hi = minMaxX(b.Data)
	return lo, hi, nil
}

func (b BarMark) yDomain() (lo, hi float64) {
	lo, hi = minMaxY(b.Data)
	return min(lo, 0), max(hi, 0) // bars anchor at zero
}

func (b BarMark) draw(p Plot) {
	if len(b.Data) == 0 {
		return
	}
	col := p.th.color(p.series, b.Color)
	frac := b.Width
	if frac <= 0 {
		frac = 0.7
	}
	slot := slotWidth(b.Data, p)
	bw := slot * frac
	y0 := p.py(0)
	for _, d := range b.Data {
		cx := p.px(d.X)
		yTop := y0 + (p.py(d.Y)-y0)*p.T // grow from the baseline
		top, bot := yTop, y0
		if top > bot {
			top, bot = bot, top
		}
		r := geom.RectXYWH(cx-bw/2, top, bw, bot-top)
		corner := b.Corner
		if corner == 0 {
			corner = min(bw*0.16, 7)
		}
		if r.Dy() < corner*2 { // avoid an over-rounded sliver
			corner = r.Dy() / 2
		}
		p.Canvas.FillRRect(r, corner, colorOr(d.Color, col))
	}
}

// colorOr returns c when it has a non-zero alpha, else the fallback.
func colorOr(c, fallback paint.Color) paint.Color {
	if c.A > 0 {
		return c
	}
	return fallback
}

// slotWidth is the pixel width available to one bar's slot: a band's bandwidth,
// or the smallest pixel gap between adjacent numeric points.
func slotWidth(d []Datum, p Plot) float32 {
	if bd, ok := p.X.(bander); ok {
		return bd.Bandwidth() * p.Area.Dx()
	}
	if len(d) < 2 {
		return p.Area.Dx() * 0.5
	}
	minGap := p.Area.Dx()
	for i := 1; i < len(d); i++ {
		if g := abs(p.px(d[i].X) - p.px(d[i-1].X)); g > 0 && g < minGap {
			minGap = g
		}
	}
	return minGap
}

func minMaxX(d []Datum) (lo, hi float64) {
	lo, hi = d[0].X, d[0].X
	for _, x := range d {
		lo, hi = min(lo, x.X), max(hi, x.X)
	}
	return
}

func minMaxY(d []Datum) (lo, hi float64) {
	lo, hi = d[0].Y, d[0].Y
	for _, x := range d {
		lo, hi = min(lo, x.Y), max(hi, x.Y)
	}
	return
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (b BarMark) seriesData() []Datum    { return b.Data }
func (b BarMark) baseColor() paint.Color { return b.Color }
