package chart

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// drawSelection highlights the selected datum with a crosshair, an emphasized
// marker, and a floating value tooltip.
func drawSelection(c paint.Canvas, area geom.Rect, xs, ys Scale, d Datum, col paint.Color, xaxis, yaxis Axis, th chartTheme, p *paint.Painter) {
	x := area.Min.X + xs.Map(d.X)*area.Dx()
	y := area.Max.Y - ys.Map(d.Y)*area.Dy()

	c.Line(geom.Pt{X: x, Y: area.Min.Y}, geom.Pt{X: x, Y: area.Max.Y}, 1, th.grid)
	dot(c, x, y, 13, halo(th)) // ring
	dot(c, x, y, 8, col)

	drawTooltip(c, p, area, geom.Pt{X: x, Y: y}, selectionLabel(xs, xaxis, d), yaxis.label(ys, Tick{Value: d.Y}), th)
}

// selectionLabel is the tooltip's first line: what the selected datum is,
// as opposed to its value. A category is its label; otherwise the x axis
// formats the position the way it formats its ticks, so a caller's
// XAxis.Format applies and a time scale reads as a date rather than as the
// epoch seconds behind it ("1,758.9M" was what a tap on a time series said).
// The one departure from the axis is detail: a time scale's ticks say "Jan 5"
// while the datum between two of them says "Jan 5 15:04".
func selectionLabel(xs Scale, xaxis Axis, d Datum) string {
	if d.Label != "" {
		return d.Label
	}
	if ts, ok := xs.(*Time); ok && xaxis.Format == nil {
		return ts.pointLabel(d.X)
	}
	return xaxis.label(xs, Tick{Value: d.X})
}

// halo is the marker ring color: near-white on light charts, near-black on dark.
func halo(th chartTheme) paint.Color {
	if th.text.R > 0.5 { // light text ⇒ dark mode
		return gray(0.12, 1)
	}
	return gray(1, 1)
}

// tooltip colors: a dark card with light text, legible over any series color.
var (
	tipBG  = paint.Color{R: 0.13, G: 0.15, B: 0.19, A: 0.97}
	tipSub = gray(0.72, 1)
	tipInk = gray(1, 1)
)

// tooltipBox places a tw x th card near anchor and keeps every edge inside
// area.
//
// It used to clamp the left edge and flip an upward overflow downward, and
// check nothing else — so a card could hang off the bottom, and off the top
// whenever the flip pushed it back out. A chart's selection is drawn after the
// plot clip is popped, so whatever escaped landed on the page: on a phone,
// the tooltip painted over the header above a scrolled chart.
//
// Clamping after placing, on both axes, is the whole fix. Preferring above-left
// keeps the old feel where there is room.
func tooltipBox(area geom.Rect, anchor geom.Pt, tw, th float32) geom.Rect {
	tx, ty := anchor.X+14, anchor.Y-th-14
	if tx+tw > area.Max.X {
		tx = anchor.X - tw - 14
	}
	if ty < area.Min.Y {
		ty = anchor.Y + 14
	}
	// Whatever the preference produced, it has to fit.
	if tx+tw > area.Max.X {
		tx = area.Max.X - tw
	}
	if tx < area.Min.X {
		tx = area.Min.X
	}
	if ty+th > area.Max.Y {
		ty = area.Max.Y - th
	}
	if ty < area.Min.Y {
		ty = area.Min.Y
	}
	return geom.RectXYWH(tx, ty, tw, th)
}

// drawTooltip renders a two-line value card near anchor, clamped inside area.
func drawTooltip(c paint.Canvas, p *paint.Painter, area geom.Rect, anchor geom.Pt, label, value string, _ chartTheme) {
	const ls, vs = float32(12), float32(15)
	mL, mV := p.MetricsIn("", ls), p.MetricsIn("", vs)
	lineL, lineV := mL.Ascent+mL.Descent, mV.Ascent+mV.Descent
	padX, padY, gap := float32(11), float32(9), float32(3)

	tw := max(p.MeasureWidthIn("", label, ls), p.MeasureWidthIn("", value, vs)) + padX*2
	th := padY*2 + lineL + gap + lineV

	box := tooltipBox(area, anchor, tw, th)
	paint.DropShadow(c, box, 9, geom.Pt{Y: 2}, 10, paint.Color{A: 0.22})
	c.FillRRect(box, 9, tipBG)
	c.TextIn("", label, geom.Pt{X: box.Min.X + padX, Y: box.Min.Y + padY + mL.Ascent}, ls, tipSub)
	c.TextIn("", value, geom.Pt{X: box.Min.X + padX, Y: box.Min.Y + padY + lineL + gap + mV.Ascent}, vs, tipInk)
}
