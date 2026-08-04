// Package healthui is the live health-dashboard showcase: a scrollable set of
// metric cards — a real-time heart rate, today's steps, weight, and sleep — each
// with a custom-painted chart, tappable through to a detail screen. It is built
// entirely from gophics widgets + one Canvas per chart, and streams live via a
// per-frame Ticker.
//
// Data comes through the Provider interface (provider.go). Desktop and web run
// the synthetic live provider; on iOS/Android the mobile bind injects a
// deviceProvider fed by HealthKit / Health Connect (Phase 2) — one Go widget
// tree, real device data. App is the root widget.
package healthui

import (
	"fmt"
	"os"
	"strconv"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Palette — a light, calm health-app look.
var (
	BG      = paint.RGB(0.95, 0.96, 0.975)
	surface = paint.RGB(1, 1, 1)
	ink     = paint.RGB(0.11, 0.12, 0.14)
	sub     = paint.RGB(0.55, 0.57, 0.62)
	white   = paint.RGB(1, 1, 1)
	heart   = paint.RGB(0.94, 0.27, 0.35)
	steps   = paint.RGB(0.18, 0.72, 0.45)
	weight  = paint.RGB(0.24, 0.52, 0.96)
	sleep   = paint.RGB(0.45, 0.40, 0.86)
)

// spec is one metric's presentation: label, unit, accent, formatting, chart.
type spec struct {
	m          Metric
	label      string
	unit       string
	caption    string
	accent     paint.Color
	cardWindow int // last N samples shown on the dashboard card (0 = all)
	fmtVal     func(float64) string
	draw       func(c paint.Canvas, size geom.Size, xs []Sample, accent paint.Color)
}

var specs = []spec{
	{HeartRate, "Heart Rate", "bpm", "live", heart, 0, fmt0, drawLineArea},
	{Steps, "Steps", "", "today", steps, 0, fmtInt, drawLineArea},
	{Weight, "Weight", "kg", "30 days", weight, 30, fmt1, drawLineArea},
	{Sleep, "Sleep", "h", "7 nights", sleep, 7, fmt1, drawBars},
}

// specFor returns a metric's spec (specs is indexed by Metric).
func specFor(m Metric) spec { return specs[m] }

// lastN returns the last n samples (n <= 0 → all).
func lastN(xs []Sample, n int) []Sample {
	if n <= 0 || n >= len(xs) {
		return xs
	}
	return xs[len(xs)-n:]
}

// --- app root: owns the provider + live ticker, gates on onboarding ---

// App is the app's root widget. Provider is the data source; when nil (the
// desktop/web default) it falls back to the synthetic live provider. The mobile
// bind packages inject a deviceProvider fed by HealthKit / App Connect.
type App struct{ Provider Provider }

func (h App) CreateState() widget.State {
	p := h.Provider
	if p == nil {
		p = newSynthProvider()
	}
	// HEALTH_VIEW skips the onboarding gate — used for screenshots and gallery
	// thumbnails. "dashboard" opens the dashboard; a metric name ("heart",
	// "weight", …) opens straight to that detail page.
	return &healthState{p: p, connected: os.Getenv("HEALTH_VIEW") != ""}
}

// metricByView maps a HEALTH_VIEW name to a metric, for deep-linking screenshots.
func metricByView(v string) (Metric, bool) {
	switch v {
	case "heart":
		return HeartRate, true
	case "steps":
		return Steps, true
	case "weight":
		return Weight, true
	case "sleep":
		return Sleep, true
	}
	return 0, false
}

type healthState struct {
	widget.StateBase[App]
	p         Provider
	connected bool
}

func (s *healthState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

// Tick advances a live synthetic source (if the provider is an Advancer) and
// repaints. A device provider isn't an Advancer — the platform pushes samples
// via callbacks — so this just repaints so pushed updates show.
func (s *healthState) Tick(dt float64) bool {
	s.SetState(func() {
		if a, ok := s.p.(Advancer); ok {
			a.Advance(dt)
		}
	})
	return true
}

func (s *healthState) Build(ctx widget.Ctx) widget.Widget {
	if !s.connected {
		return s.onboarding()
	}
	// The dashboard is the Navigator's Home so it (and pushed detail pages) can
	// reach the Nav handle. The provider lives here at the root and keeps
	// streaming regardless of which page is on top.
	home := widget.Widget(dashboard{p: s.p})
	if m, ok := metricByView(os.Getenv("HEALTH_VIEW")); ok {
		home = detailPage{p: s.p, m: m} // deep-link for screenshots
	}
	return widget.Navigator{Home: home}
}

func (s *healthState) onboarding() widget.Widget {
	connect := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.connected = true }) }},
		Child: widget.Decorated{Color: heart, Radius: 14, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(28, 14),
			Child:  widget.Text{S: "Connect " + s.p.Name(), Size: 16, Color: white},
		}},
	}
	return widget.Fill{Color: BG, Child: widget.Align{X: 0.5, Y: 0.5, Child: widget.Padding{
		All: 32,
		Child: widget.Flex{CrossAlign: layout.CrossCenter, Children: []widget.Widget{
			widget.Text{S: "♥", Size: 72, Color: heart},
			widget.Padding{Insets: geom.Insets{Top: 12}, Child: widget.Text{S: "Health", Size: 34, Color: ink}},
			widget.Padding{Insets: geom.Insets{Top: 6, Bottom: 26}, Child: widget.Text{
				S: "Connect your data to see it live.", Size: 15, Color: sub}},
			connect,
		}},
	}}}
}

// --- dashboard page (Navigator Home) ---

type dashboard struct{ p Provider }

func (d dashboard) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	children := []widget.Widget{header(d.p)}
	for _, sp := range specs {
		m := sp.m
		children = append(children, card(d.p, sp, func() { nav.Push(detailPage{p: d.p, m: m}) }))
	}
	return widget.Fill{Color: BG, Child: widget.Scroll{
		Child: widget.Padding{
			Insets: geom.InsetsSymmetric(18, 22),
			Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: children},
		},
	}}
}

func header(p Provider) widget.Widget {
	return widget.Padding{
		Insets: geom.Insets{Bottom: 18},
		Child: widget.Flex{CrossAlign: layout.CrossStart, Children: []widget.Widget{
			widget.Text{S: "Health", Size: 32, Color: ink},
			widget.Text{S: p.Name(), Size: 14, Color: sub},
		}},
	}
}

// card renders one dashboard metric card, tappable through to its detail page.
func card(p Provider, sp spec, onTap func()) widget.Widget {
	val, _ := p.Latest(sp.m)
	series := lastN(p.Series(sp.m), sp.cardWindow)

	title := widget.Row(
		widget.Text{S: sp.label, Size: 14, Color: sp.accent},
		widget.Spacer(),
		widget.Text{S: sp.caption, Size: 12, Color: sub},
	)
	value := widget.Row(
		widget.Text{S: sp.fmtVal(val.V), Size: 34, Color: ink},
		widget.Padding{Insets: geom.Insets{Left: 5, Top: 12}, Child: widget.Text{S: sp.unit, Size: 14, Color: sub}},
	)
	chart := widget.Expand(widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		sp.draw(c, size, series, sp.accent)
	}})

	body := widget.Decorated{Color: surface, Radius: 18, Child: widget.Padding{
		All:   16,
		Child: widget.Flex{CrossAlign: layout.CrossStretch, Children: []widget.Widget{title, value, chart}},
	}}
	return widget.Padding{
		Insets: geom.Insets{Bottom: 14},
		Child: widget.Sized{H: 170, Child: widget.Interactive{
			Handler: widget.Handler{OnTap: onTap},
			Child:   body,
		}},
	}
}

// --- charts (custom paint) ---

// drawLineArea plots xs as a filled area under a 2px line, with a dot at the
// latest point. Values map to y; index maps to x, so the live heart-rate window
// scrolls left as old samples drop and new ones append.
func drawLineArea(c paint.Canvas, size geom.Size, xs []Sample, accent paint.Color) {
	if len(xs) < 2 {
		return
	}
	lo, hi := xs[0].V, xs[0].V
	for _, s := range xs {
		lo, hi = min(lo, s.V), max(hi, s.V)
	}
	if hi-lo < 1e-6 {
		hi = lo + 1
	}
	const pad = 8
	px := func(i int) float32 { return pad + float32(i)/float32(len(xs)-1)*(size.W-2*pad) }
	py := func(v float64) float32 { return size.H - pad - float32((v-lo)/(hi-lo))*(size.H-2*pad) }

	area := paint.NewPath()
	area.MoveTo(geom.Pt{X: px(0), Y: size.H})
	for i, s := range xs {
		area.LineTo(geom.Pt{X: px(i), Y: py(s.V)})
	}
	area.LineTo(geom.Pt{X: px(len(xs) - 1), Y: size.H})
	area.Close()
	c.FillPath(area, accent.WithAlpha(0.22))

	line := paint.NewPath()
	line.MoveTo(geom.Pt{X: px(0), Y: py(xs[0].V)})
	for i, s := range xs {
		line.LineTo(geom.Pt{X: px(i), Y: py(s.V)})
	}
	c.StrokePath(line, 2, accent)

	last := len(xs) - 1
	c.FillRRect(geom.RectXYWH(px(last)-3.5, py(xs[last].V)-3.5, 7, 7), 3.5, accent)
}

// drawBars plots xs as rounded bars scaled to the max value — used for sleep.
func drawBars(c paint.Canvas, size geom.Size, xs []Sample, accent paint.Color) {
	if len(xs) == 0 {
		return
	}
	hi := xs[0].V
	for _, s := range xs {
		hi = max(hi, s.V)
	}
	if hi < 1e-6 {
		hi = 1
	}
	const pad, gap = 8, 7
	n := len(xs)
	bw := (size.W - 2*pad - gap*float32(n-1)) / float32(n)
	for i, s := range xs {
		bh := float32(s.V/hi) * (size.H - 2*pad)
		x := pad + float32(i)*(bw+gap)
		c.FillRRect(geom.RectXYWH(x, size.H-pad-bh, bw, bh), 4, accent.WithAlpha(0.85))
	}
}

// --- value formatting ---

func fmt0(v float64) string { return fmt.Sprintf("%.0f", v) }
func fmt1(v float64) string { return fmt.Sprintf("%.1f", v) }

func fmtInt(v float64) string {
	n := int(v)
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	out := ""
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(s[i])
	}
	if neg {
		return "-" + out
	}
	return out
}
