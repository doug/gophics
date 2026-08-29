package chart

import (
	"github.com/doug/gophics/intl"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Axis configures one axis of a chart.
type Axis struct {
	Hide   bool                 // omit the axis and its labels entirely
	Grid   bool                 // X axis: draw vertical gridlines at the ticks. (Horizontal Y gridlines are always drawn as chart chrome, so this field has no effect on the Y axis.)
	Ticks  int                  // target tick count; 0 → a sensible default
	Format func(float64) string // tick label formatter for numeric values

	// loc formats the default numeric labels. Set by Chart.Build from
	// ctx.IntlLocale(), not by the app: a chart's axis numbers should follow
	// the device like every other number on screen, and asking each caller to
	// pass a locale would mean most of them forget and a German chart would go
	// on reading 1,234.5. An app that wants something else sets Format.
	loc intl.Locale
}

// withLocale returns a copy that formats numbers for loc.
func (a Axis) withLocale(loc intl.Locale) Axis {
	a.loc = loc
	return a
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
	return fmtNumber(t.Value, a.loc)
}

// drawYAxis draws horizontal gridlines and right-aligned value labels.
func drawYAxis(c paint.Canvas, area geom.Rect, ys Scale, ax Axis, th chartTheme, p *paint.Painter) {
	if ax.Hide {
		return
	}
	met := p.MetricsIn("", labelSize)
	baseline := (met.Ascent - met.Descent) / 2 // vertical-center offset onto the tick
	for _, t := range ys.Ticks(ax.tickCount(5)) {
		y := area.Max.Y - t.Pos*area.Dy()
		c.Line(geom.Pt{X: area.Min.X, Y: y}, geom.Pt{X: area.Max.X, Y: y}, 1, th.grid)
		lab := ax.label(t)
		w := p.MeasureWidthIn("", lab, labelSize)
		c.TextIn("", lab, geom.Pt{X: area.Min.X - 8 - w, Y: y + baseline}, labelSize, th.text)
	}
}

// drawXAxis draws the baseline, optional vertical gridlines, and centered labels.
// xLabelGap is the minimum space left between two x-axis labels. Below this
// they read as one run of characters even when they do not technically touch.
const xLabelGap = 8

// visibleXLabels decides which x-axis tick labels can be drawn without running
// into each other, walking left to right and keeping a tick only when its label
// clears the last one kept.
//
// Labels are centred on their tick, so their width — not the tick spacing —
// decides whether they collide: six ticks fit comfortably until the labels are
// dates or names, and then they overlap into an unreadable smear. Dropping
// every other label keeps the axis legible at any width, and the gridlines
// still mark the positions of the ticks that lost their label.
//
// The first tick always keeps its label, so an axis never comes back empty.
func visibleXLabels(ticks []Tick, area, bounds geom.Rect, ax Axis, measure func(string) float32) []bool {
	show := make([]bool, len(ticks))
	prevRight := float32(0)
	first := true
	for i, t := range ticks {
		w := measure(ax.label(t))
		left := xLabelLeft(area.Min.X+t.Pos*area.Dx(), w, bounds)
		if first || left >= prevRight+xLabelGap {
			show[i] = true
			prevRight = left + w
			first = false
		}
	}
	return show
}

// xLabelLeft is where a tick label of width w starts: centred under the tick,
// but pulled back inside bounds at the ends.
//
// A tick at the far right of the plot sits on area.Max.X, so a centred label
// runs half its width past it — into a right margin of 14pt, which is narrower
// than any date. The overflow used to be clipped by the chart's own box and the
// last label lost its tail: "Nov '25" rendered as "Nov '2". Clamping is what
// the label needs rather than a wider margin, since the width depends on the
// formatter and nothing reserves space per-tick.
//
// The same clamp runs in visibleXLabels, so overlap is judged on where labels
// actually land rather than where an unclamped one would have.
func xLabelLeft(tickX, w float32, bounds geom.Rect) float32 {
	left := tickX - w/2
	if right := left + w; right > bounds.Max.X {
		left = bounds.Max.X - w
	}
	if left < bounds.Min.X {
		left = bounds.Min.X
	}
	return left
}

func drawXAxis(c paint.Canvas, area, bounds geom.Rect, xs Scale, ax Axis, th chartTheme, p *paint.Painter) {
	if ax.Hide {
		return
	}
	met := p.MetricsIn("", labelSize)
	ticks := xs.Ticks(ax.tickCount(6))
	show := visibleXLabels(ticks, area, bounds, ax, func(lab string) float32 {
		return p.MeasureWidthIn("", lab, labelSize)
	})
	for i, t := range ticks {
		x := area.Min.X + t.Pos*area.Dx()
		if ax.Grid {
			c.Line(geom.Pt{X: x, Y: area.Min.Y}, geom.Pt{X: x, Y: area.Max.Y}, 1, th.grid)
		}
		if !show[i] {
			continue // its neighbour is already there; two labels on top of each
			// other read as neither
		}
		lab := ax.label(t)
		w := p.MeasureWidthIn("", lab, labelSize)
		lx := xLabelLeft(x, w, bounds)
		c.TextIn("", lab, geom.Pt{X: lx, Y: area.Max.Y + met.Ascent + 6}, labelSize, th.text)
	}
	c.Line(geom.Pt{X: area.Min.X, Y: area.Max.Y}, geom.Pt{X: area.Max.X, Y: area.Max.Y}, 1, th.axis)
}
