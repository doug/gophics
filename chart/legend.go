package chart

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

const legendSize = float32(13)

// markColor resolves a mark's legend swatch color: its override, else the series
// color for its position.
func markColor(mk Mark, th theme, i int) paint.Color {
	if sc, ok := mk.(selectable); ok {
		return th.color(i, sc.baseColor())
	}
	return th.series[i%len(th.series)]
}

// drawLegend lays out the color key as a single row centered above the plot.
func drawLegend(c paint.Canvas, area geom.Rect, entries []legendEntry, th theme, p *paint.Painter) {
	met := p.Metrics(legendSize)
	sw := legendSize * 0.85 // swatch size
	const gap, itemGap = 6, 20

	total := float32(0)
	for i, e := range entries {
		total += sw + gap + p.MeasureWidth(e.label, legendSize)
		if i < len(entries)-1 {
			total += itemGap
		}
	}
	x := area.Min.X + (area.Dx()-total)/2
	if x < area.Min.X {
		x = area.Min.X
	}
	y := area.Min.Y - met.LineHeight()/2 - 8 // vertically centered in the reserved band

	for _, e := range entries {
		c.FillRRect(geom.RectXYWH(x, y-sw/2, sw, sw), 3, e.color)
		x += sw + gap
		c.Text(e.label, geom.Pt{X: x, Y: y + met.Ascent/2 - met.Descent/2}, legendSize, th.text)
		x += p.MeasureWidth(e.label, legendSize) + itemGap
	}
}
