package chart

import (
	"math"
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Chart is a widget that renders a set of marks over shared axes. Scales are
// inferred from the marks' data unless X/Y are set explicitly.
type Chart struct {
	Marks   []Mark
	X, Y    Scale // optional; inferred from the marks when nil
	XAxis   Axis
	YAxis   Axis
	Legend  bool // show a color key for named marks
	Animate bool // grow marks in on mount (skipped under reduce-motion)

	// Chrome overrides the axis/label/gridline colors; each is used only when its
	// alpha is non-zero, so the zero Chart keeps the scheme-adaptive default.
	// Set these from a design theme (e.g. LabelColor: th.Text, AxisColor: th.Muted,
	// GridColor: th.Border) so a chart matches the surrounding app in light and dark.
	LabelColor paint.Color // tick labels, axis titles, legend text
	AxisColor  paint.Color // axis baselines
	GridColor  paint.Color // gridlines
	// Palette overrides the categorical series colors used for marks and pie
	// slices that don't set their own color; nil keeps the scheme palette.
	Palette []paint.Color
}

// chrome applies any non-zero color overrides on top of the scheme-resolved
// theme, so callers can theme a chart's axes, labels, and series palette.
func (w Chart) chrome(t chartTheme) chartTheme {
	if w.LabelColor.A > 0 {
		t.text = w.LabelColor
	}
	if w.AxisColor.A > 0 {
		t.axis = w.AxisColor
	}
	if w.GridColor.A > 0 {
		t.grid = w.GridColor
	}
	if len(w.Palette) > 0 {
		t.series = w.Palette
	}
	return t
}

// legendEntry is one color-key row.
type legendEntry struct {
	label string
	color paint.Color
}

func (Chart) CreateState() widget.State { return &chartState{} }

// stateHook lets tests observe the mounted chart state.
var stateHook func(*chartState)

type chartState struct {
	widget.StateBase[Chart]
	ctx  widget.Ctx
	anim *anim.Controller
	t    float32

	// Selection: the primary data series and the currently-selected index
	// (-1 = none), plus the last drawn plot area so the press handler can map a
	// pointer back to a datum.
	sel int
	// pressPos is where the pointer went down, held so OnTap can select at
	// that point: OnTap carries no position of its own.
	pressPos geom.Pt
	xs, ys   Scale
	area     geom.Rect
	primary  []Datum
	selCol   paint.Color
	legend   []legendEntry
}

func (s *chartState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.t = 1
	s.sel = -1
	if s.W().Animate && !ctx.ReduceMotion() {
		s.t = 0
		s.anim = &anim.Controller{Duration: 600 * time.Millisecond, Curve: anim.EaseOut,
			OnChange: func() { s.t = s.anim.Value(); s.ctx.Invalidate() }}
		ctx.AddTicker(s.anim)
		s.anim.Forward()
	}
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *chartState) Dispose() {
	if s.anim != nil {
		s.ctx.RemoveTicker(s.anim)
	}
}

func (s *chartState) Build(ctx widget.Ctx) widget.Widget {
	w := s.W()
	xs, ys := resolveScales(w)
	th := w.chrome(themeFor(ctx.DarkMode()))
	p := ctx.Painter()
	s.xs, s.ys = xs, ys

	// The primary series is the first data-bearing mark — the one a press
	// selects against. Meanwhile collect legend entries and count grouped bars.
	s.primary, s.selCol, s.legend = nil, paint.Color{}, nil
	barGroups := 0
	for i, mk := range w.Marks {
		if sd, ok := mk.(selectable); ok && len(sd.seriesData()) > 0 && s.primary == nil {
			s.primary = sd.seriesData()
			s.selCol = th.color(i, sd.baseColor())
		}
		if _, ok := mk.(BarMark); ok {
			barGroups++
		}
		if le, ok := mk.(legender); ok {
			s.legend = append(s.legend, le.legendEntries(th)...)
		} else if n, ok := mk.(named); ok && n.markName() != "" {
			s.legend = append(s.legend, legendEntry{n.markName(), markColor(mk, th, i)})
		}
	}
	m := margins(p, w, ys)
	if w.Legend && len(s.legend) > 0 {
		m.Top += float32(legendRows(s.legend, p))*p.MetricsIn("", legendSize).LineHeight() + 10
	}

	// Clipped to the widget's own box. The marks are clipped to the plot area
	// below, but the selection — crosshair, dot and tooltip — is drawn after
	// that clip is popped, and the legend after that, so an unclipped canvas
	// let a chart paint onto whatever surrounded it. On a scrolled page that
	// was the header above it.
	canvas := widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		area := m.Inset(geom.RectFromSize(size))
		s.area = area
		if area.IsEmpty() {
			return
		}
		drawYAxis(c, area, ys, w.YAxis, th, p)
		drawXAxis(c, area, xs, w.XAxis, th, p)

		pl := plot{Area: area, X: xs, Y: ys, Canvas: c, th: th, T: s.t, groups: 1}
		c.PushClip(area)
		gi := 0
		for i, mk := range w.Marks {
			pl.series = i
			if _, ok := mk.(BarMark); ok {
				pl.group, pl.groups = gi, barGroups
				gi++
			} else {
				pl.group, pl.groups = 0, 1
			}
			mk.draw(pl)
		}
		c.PopClip()

		if s.sel >= 0 && s.sel < len(s.primary) {
			drawSelection(c, area, xs, ys, s.primary[s.sel], s.selCol, w.YAxis, th, p)
		}
		if w.Legend && len(s.legend) > 0 {
			drawLegend(c, area, s.legend, th, p)
		}
	}}

	var root widget.Widget = canvas
	if len(s.primary) > 0 {
		root = widget.Interactive{
			Handler: widget.Handler{
				// Selection commits on tap rather than on press. A finger
				// landing on a chart is usually the start of a scroll, and
				// selecting on touch-down strands a tooltip and crosshair when
				// the scroll then takes the gesture. Crossing the tap slop
				// cancels the pending tap, so only a real tap selects.
				OnPress: func(pos geom.Pt) { s.pressPos = pos },
				OnTap:   func() { s.selectAt(s.pressPos) },
				OnDrag:  func(pos, _ geom.Pt) { s.selectAt(pos) },
				// Scrubbing picks the datum nearest in x, so it is a horizontal
				// gesture; a vertical drag belongs to the enclosing scroll.
				DragAxis: widget.DragHorizontal,
			},
			Child: canvas,
		}
	}
	if label := w.semanticsLabel(); label != "" {
		root = widget.Semantics{Role: layout.RoleGroup, Label: label, Child: root}
	}
	return root
}

// selectAt selects the primary datum whose x is nearest the pointer.
func (s *chartState) selectAt(pos geom.Pt) {
	if len(s.primary) == 0 || s.area.IsEmpty() {
		return
	}
	best, bestDist := -1, float32(1e9)
	for i, d := range s.primary {
		x := s.area.Min.X + s.xs.Map(d.X)*s.area.Dx()
		if dd := abs(x - pos.X); dd < bestDist {
			bestDist, best = dd, i
		}
	}
	if best != s.sel {
		s.sel = best
		s.ctx.Invalidate()
	}
}

// selectable marks expose their data (and base color) for press selection.
type selectable interface {
	seriesData() []Datum
	baseColor() paint.Color
}

// margins reserves space for the axis labels: the widest y-label on the left and
// a line of x-labels below.
func margins(p *paint.Painter, w Chart, ys Scale) geom.Insets {
	in := geom.Insets{Top: 10, Right: 14, Bottom: 8, Left: 10}
	if !w.YAxis.Hide {
		maxw := float32(0)
		for _, t := range ys.Ticks(w.YAxis.tickCount(5)) {
			if ww := p.MeasureWidthIn("", w.YAxis.label(t), labelSize); ww > maxw {
				maxw = ww
			}
		}
		in.Left = maxw + 14
	}
	if !w.XAxis.Hide {
		in.Bottom = p.MetricsIn("", labelSize).LineHeight() + 10
	}
	return in
}

// resolveScales returns the X and Y scales, inferring them from the marks when
// the caller left them nil.
func resolveScales(w Chart) (xs, ys Scale) {
	xs, ys = w.X, w.Y
	if xs != nil && ys != nil {
		return xs, ys
	}
	var (
		xlo, xhi = math.Inf(1), math.Inf(-1)
		ylo, yhi = math.Inf(1), math.Inf(-1)
		bandCats []string
	)
	for _, mk := range w.Marks {
		lo, hi, cs := mk.xDomain()
		if cs != nil {
			bandCats = cs
		}
		xlo, xhi = math.Min(xlo, lo), math.Max(xhi, hi)
		ylo2, yhi2 := mk.yDomain()
		ylo, yhi = math.Min(ylo, ylo2), math.Max(yhi, yhi2)
	}
	if xhi < xlo { // no numeric X anywhere
		xlo, xhi = 0, 1
	}
	if yhi < ylo {
		ylo, yhi = 0, 1
	}
	if xs == nil {
		if bandCats != nil {
			xs = NewBand(bandCats)
		} else {
			xs = NewLinear(xlo, xhi)
		}
	}
	if ys == nil {
		ys = NewLinear(ylo, yhi)
	}
	return xs, ys
}
