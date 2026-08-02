package chart

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// drawSelection highlights the selected datum with a crosshair, an emphasized
// marker, and a floating value tooltip.
func drawSelection(c paint.Canvas, area geom.Rect, xs, ys Scale, d Datum, col paint.Color, yaxis Axis, th theme, p *paint.Painter) {
	x := area.Min.X + xs.Map(d.X)*area.Dx()
	y := area.Max.Y - ys.Map(d.Y)*area.Dy()

	c.Line(geom.Pt{X: x, Y: area.Min.Y}, geom.Pt{X: x, Y: area.Max.Y}, 1, th.grid)
	dot(c, x, y, 13, halo(th)) // ring
	dot(c, x, y, 8, col)

	label := d.Label
	if label == "" {
		label = fmtNumber(d.X)
	}
	drawTooltip(c, p, area, geom.Pt{X: x, Y: y}, label, yaxis.label(Tick{Value: d.Y}), th)
}

// halo is the marker ring color: near-white on light charts, near-black on dark.
func halo(th theme) paint.Color {
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

// drawTooltip renders a two-line value card near anchor, clamped inside area.
func drawTooltip(c paint.Canvas, p *paint.Painter, area geom.Rect, anchor geom.Pt, label, value string, _ theme) {
	const ls, vs = float32(12), float32(15)
	mL, mV := p.Metrics(ls), p.Metrics(vs)
	lineL, lineV := mL.Ascent+mL.Descent, mV.Ascent+mV.Descent
	padX, padY, gap := float32(11), float32(9), float32(3)

	tw := maxf(p.MeasureWidth(label, ls), p.MeasureWidth(value, vs)) + padX*2
	th := padY*2 + lineL + gap + lineV

	tx, ty := anchor.X+14, anchor.Y-th-14
	if tx+tw > area.Max.X {
		tx = anchor.X - tw - 14
	}
	if tx < area.Min.X {
		tx = area.Min.X
	}
	if ty < area.Min.Y {
		ty = anchor.Y + 14
	}
	box := geom.RectXYWH(tx, ty, tw, th)
	paint.DropShadow(c, box, 9, geom.Pt{Y: 2}, 10, paint.Color{A: 0.22})
	c.FillRRect(box, 9, tipBG)
	c.Text(label, geom.Pt{X: tx + padX, Y: ty + padY + mL.Ascent}, ls, tipSub)
	c.Text(value, geom.Pt{X: tx + padX, Y: ty + padY + lineL + gap + mV.Ascent}, vs, tipInk)
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
