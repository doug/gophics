package chart

import (
	"math"
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// Chart is a widget that renders a set of marks over shared axes. Scales are
// inferred from the marks' data unless X/Y are set explicitly.
type Chart struct {
	Marks   []Mark
	X, Y    Scale // optional; inferred from the marks when nil
	XAxis   Axis
	YAxis   Axis
	Animate bool // grow marks in on mount (skipped under reduce-motion)
}

func (Chart) CreateState() widget.State { return &chartState{} }

type chartState struct {
	widget.StateBase[Chart]
	ctx  widget.Ctx
	anim *anim.Controller
	t    float32
}

func (s *chartState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.t = 1
	if s.W().Animate && !ctx.ReduceMotion() {
		s.t = 0
		s.anim = &anim.Controller{Duration: 600 * time.Millisecond, Curve: anim.EaseOut,
			OnChange: func() { s.t = s.anim.Value(); s.ctx.Invalidate() }}
		ctx.AddTicker(s.anim)
		s.anim.Forward()
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
	th := themeFor(ctx.DarkMode())
	p := ctx.Painter()
	m := margins(p, w, ys)

	return widget.Canvas{Clip: false, Draw: func(c paint.Canvas, size geom.Size) {
		area := m.Inset(geom.RectFromSize(size))
		if area.IsEmpty() {
			return
		}
		drawYAxis(c, area, ys, w.YAxis, th, p)
		drawXAxis(c, area, xs, w.XAxis, th, p)

		pl := Plot{Area: area, X: xs, Y: ys, Canvas: c, th: th, T: s.t}
		c.PushClip(area)
		for i, mk := range w.Marks {
			pl.series = i
			mk.draw(pl)
		}
		c.PopClip()
	}}
}

// margins reserves space for the axis labels: the widest y-label on the left and
// a line of x-labels below.
func margins(p *paint.Painter, w Chart, ys Scale) geom.Insets {
	in := geom.Insets{Top: 10, Right: 14, Bottom: 8, Left: 10}
	if !w.YAxis.Hide {
		maxw := float32(0)
		for _, t := range ys.Ticks(w.YAxis.tickCount(5)) {
			if ww := p.MeasureWidth(w.YAxis.label(t), labelSize); ww > maxw {
				maxw = ww
			}
		}
		in.Left = maxw + 14
	}
	if !w.XAxis.Hide {
		in.Bottom = p.Metrics(labelSize).LineHeight() + 10
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
