package chart

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

const legendSize = float32(13)

// legender marks contribute one or more legend rows (e.g. a pie's slices).
type legender interface {
	legendEntries(th theme) []legendEntry
}

// markColor resolves a mark's legend swatch color: its override, else the series
// color for its position.
func markColor(mk Mark, th theme, i int) paint.Color {
	if sc, ok := mk.(selectable); ok {
		return th.color(i, sc.baseColor())
	}
	return th.series[i%len(th.series)]
}

const (
	legendSwatchGap = float32(6)
	legendItemGap   = float32(18)
	legendTopPad    = float32(8)
)

func legendItemWidth(e legendEntry, sw float32, p *paint.Painter) float32 {
	return sw + legendSwatchGap + p.MeasureWidth(e.label, legendSize)
}

// legendRows estimates how many rows the key needs (capped at 2), used to
// reserve top margin before the plot width is known.
func legendRows(entries []legendEntry, p *paint.Painter) int {
	sw := legendSize * 0.85
	total := float32(0)
	for _, e := range entries {
		total += legendItemWidth(e, sw, p) + legendItemGap
	}
	rows := int(total/300) + 1
	if rows > 2 {
		rows = 2
	}
	return rows
}

// drawLegend lays the color key out left-to-right above the plot, wrapping to a
// new row when an item would overflow the plot width.
func drawLegend(c paint.Canvas, area geom.Rect, entries []legendEntry, th theme, p *paint.Painter) {
	met := p.Metrics(legendSize)
	sw := legendSize * 0.85
	lineGap := met.LineHeight()

	x, row := area.Min.X, 0
	for _, e := range entries {
		iw := legendItemWidth(e, sw, p)
		if x > area.Min.X && x+iw > area.Max.X {
			row++
			x = area.Min.X
		}
		y := legendTopPad + float32(row)*lineGap + met.Ascent
		c.FillRRect(geom.RectXYWH(x, y-met.Ascent*0.72, sw, sw), 3, e.color)
		c.TextIn("", e.label, geom.Pt{X: x + sw + legendSwatchGap, Y: y}, legendSize, th.text)
		x += iw + legendItemGap
	}
}
