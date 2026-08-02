package chart

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Axis configures one axis of a chart.
type Axis struct {
	Hide   bool                 // omit the axis and its labels entirely
	Grid   bool                 // draw gridlines for this axis (Y grid is on by default)
	Ticks  int                  // target tick count; 0 → a sensible default
	Format func(float64) string // tick label formatter for numeric values
}

const labelSize = float32(12)

func (a Axis) tickCount(def int) int {
	if a.Ticks > 0 {
		return a.Ticks
	}
	return def
}

// label renders a tick's text: a band category as-is, else the axis formatter,
// else a compact number.
func (a Axis) label(t Tick) string {
	if t.Label != "" {
		return t.Label
	}
	if a.Format != nil {
		return a.Format(t.Value)
	}
	return fmtNumber(t.Value)
}

// drawYAxis draws horizontal gridlines and right-aligned value labels.
func drawYAxis(c paint.Canvas, area geom.Rect, ys Scale, ax Axis, th theme, p *paint.Painter) {
	if ax.Hide {
		return
	}
	met := p.Metrics(labelSize)
	baseline := (met.Ascent - met.Descent) / 2 // vertical-center offset onto the tick
	for _, t := range ys.Ticks(ax.tickCount(5)) {
		y := area.Max.Y - t.Pos*area.Dy()
		c.Line(geom.Pt{X: area.Min.X, Y: y}, geom.Pt{X: area.Max.X, Y: y}, 1, th.grid)
		lab := ax.label(t)
		w := p.MeasureWidth(lab, labelSize)
		c.Text(lab, geom.Pt{X: area.Min.X - 8 - w, Y: y + baseline}, labelSize, th.text)
	}
}

// drawXAxis draws the baseline, optional vertical gridlines, and centered labels.
func drawXAxis(c paint.Canvas, area geom.Rect, xs Scale, ax Axis, th theme, p *paint.Painter) {
	if ax.Hide {
		return
	}
	met := p.Metrics(labelSize)
	for _, t := range xs.Ticks(ax.tickCount(6)) {
		x := area.Min.X + t.Pos*area.Dx()
		if ax.Grid {
			c.Line(geom.Pt{X: x, Y: area.Min.Y}, geom.Pt{X: x, Y: area.Max.Y}, 1, th.grid)
		}
		lab := ax.label(t)
		w := p.MeasureWidth(lab, labelSize)
		c.Text(lab, geom.Pt{X: x - w/2, Y: area.Max.Y + met.Ascent + 6}, labelSize, th.text)
	}
	c.Line(geom.Pt{X: area.Min.X, Y: area.Max.Y}, geom.Pt{X: area.Max.X, Y: area.Max.Y}, 1, th.axis)
}
