package healthui

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// detailPage is a per-metric drill-down pushed onto the Navigator from the
// dashboard. It reads the shared provider and repaints itself each frame so
// live updates (advanced by the root ticker) show here too.
type detailPage struct {
	p Provider
	m Metric
}

func (detailPage) CreateState() widget.State { return &detailState{} }

type detailState struct {
	widget.StateBase[detailPage]
	rangeIdx int
}

func (s *detailState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

// Tick repaints only — the root ticker owns advancing the provider, so the
// detail never double-advances it.
func (s *detailState) Tick(dt float64) bool { s.SetState(nil); return true }

type rangeOpt struct {
	label string
	n     int // last n samples; 0 = all
}

// rangesFor returns the range tabs for a metric, or nil for metrics with a
// single natural window (live heart rate, today's steps).
func rangesFor(m Metric) []rangeOpt {
	switch m {
	case Weight, Sleep:
		return []rangeOpt{{"Week", 7}, {"Month", 30}, {"All", 0}}
	}
	return nil
}

func (s *detailState) Build(ctx widget.Ctx) widget.Widget {
	cfg := s.W()
	p, m := cfg.p, cfg.m
	sp := specFor(m)
	th := theme.Of(ctx)
	accent := th.ChartAt(sp.accentIdx)
	nav := widget.MustOf[widget.Nav](ctx)

	ranges := rangesFor(m)
	series := p.Series(m)
	if len(ranges) > 0 {
		if s.rangeIdx < 0 || s.rangeIdx >= len(ranges) {
			s.rangeIdx = 0
		}
		series = lastN(series, ranges[s.rangeIdx].n)
	}
	val, _ := p.Latest(m)
	lo, avg, hi := stats(series)

	back := widget.Interactive{
		Handler: widget.Handler{OnTap: nav.Pop},
		Child:   widget.Text{S: "‹  Back", Size: 15, Color: accent},
	}
	head := widget.Flex{CrossAlign: layout.CrossStart, Children: []widget.Widget{
		widget.Padding{Insets: geom.Insets{Bottom: 16}, Child: back},
		widget.Text{S: sp.label, Size: 15, Color: accent},
		widget.Row(
			widget.Text{S: sp.fmtVal(val.V), Size: 40, Color: th.Text},
			widget.Padding{Insets: geom.Insets{Left: 6, Top: 16}, Child: widget.Text{S: sp.unit, Size: 15, Color: th.Muted}},
		),
	}}

	kids := []widget.Widget{head}

	if len(ranges) > 0 {
		chips := make([]widget.Widget, len(ranges))
		for i, r := range ranges {
			idx := i
			chips[i] = chip(th, r.label, i == s.rangeIdx, accent, func() {
				s.SetState(func() { s.rangeIdx = idx })
			})
		}
		kids = append(kids, widget.Padding{Insets: geom.Insets{Top: 14}, Child: widget.Row(chips...)})
	}

	kids = append(kids, widget.Padding{
		Insets: geom.Insets{Top: 14, Bottom: 18},
		Child: widget.Sized{H: 230, Child: widget.Decorated{Color: th.Surface, Radius: th.Radius + 8, BorderColor: th.Border, BorderWidth: 1, Child: widget.Padding{
			All: 14,
			Child: widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
				sp.draw(c, size, series, accent)
			}},
		}}},
	})

	kids = append(kids, widget.Row(
		statBlock(th, "Min", sp.fmtVal(lo)),
		statBlock(th, "Avg", sp.fmtVal(avg)),
		statBlock(th, "Max", sp.fmtVal(hi)),
	))

	return widget.Fill{Color: th.Bg, Child: widget.Scroll{Child: widget.Padding{
		Insets: geom.InsetsSymmetric(18, 22),
		Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: kids},
	}}}
}

// chip is a pill-shaped range tab.
func chip(th theme.Theme, label string, selected bool, accent paint.Color, onTap func()) widget.Widget {
	fg, bgc := th.Muted, th.SurfaceHover
	if selected {
		fg, bgc = th.OnPrimary, accent
	}
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Padding{Insets: geom.Insets{Right: 8}, Child: widget.Decorated{
			Color: bgc, Radius: 10,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(14, 7), Child: widget.Text{S: label, Size: 13, Color: fg}},
		}},
	}
}

// statBlock is one equal-width Min/Avg/Max cell.
func statBlock(th theme.Theme, label, value string) widget.Widget {
	return widget.Expand(widget.Flex{CrossAlign: layout.CrossStart, Children: []widget.Widget{
		widget.Text{S: value, Size: 22, Color: th.Text},
		widget.Text{S: label, Size: 12, Color: th.Muted},
	}})
}

// stats returns the min, mean, and max of the samples' values.
func stats(xs []Sample) (lo, avg, hi float64) {
	if len(xs) == 0 {
		return 0, 0, 0
	}
	lo, hi = xs[0].V, xs[0].V
	sum := 0.0
	for _, s := range xs {
		lo, hi = min(lo, s.V), max(hi, s.V)
		sum += s.V
	}
	return lo, sum / float64(len(xs)), hi
}
