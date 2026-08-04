// Command health is a live health-dashboard showcase: a scrollable set of metric
// cards — a real-time heart rate, today's steps, weight, and sleep — each with a
// custom-painted chart. It is built entirely from gophics widgets + one Canvas
// per chart, and streams live via a per-frame Ticker.
//
// Data comes through the Provider interface (provider.go). Desktop and web run
// the synthetic live provider; on iOS/Android the same UI is meant to bind to
// HealthKit / Health Connect (Phase 2) — one Go widget tree, real device data.
//
//	go run ./examples/health
package main

import (
	"fmt"
	"log"
	"strconv"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Palette — a light, calm health-app look.
var (
	bg     = paint.RGB(0.95, 0.96, 0.975)
	card   = paint.RGB(1, 1, 1)
	ink    = paint.RGB(0.11, 0.12, 0.14)
	sub    = paint.RGB(0.55, 0.57, 0.62)
	heart  = paint.RGB(0.94, 0.27, 0.35)
	steps  = paint.RGB(0.18, 0.72, 0.45)
	weight = paint.RGB(0.24, 0.52, 0.96)
	sleep  = paint.RGB(0.45, 0.40, 0.86)
)

// spec is one dashboard card: which metric, how to label and draw it.
type spec struct {
	m       Metric
	label   string
	unit    string
	caption string
	accent  paint.Color
	fmtVal  func(float64) string
	draw    func(c paint.Canvas, size geom.Size, xs []Sample, accent paint.Color)
}

var specs = []spec{
	{HeartRate, "Heart Rate", "bpm", "live", heart, fmt0, drawLineArea},
	{Steps, "Steps", "", "today", steps, fmtInt, drawLineArea},
	{Weight, "Weight", "kg", "30 days", weight, fmt1, drawLineArea},
	{Sleep, "Sleep", "h", "7 nights", sleep, fmt1, drawBars},
}

type Health struct{}

func (Health) CreateState() widget.State { return &healthState{p: newSynthProvider()} }

type healthState struct {
	widget.StateBase[Health]
	p *synthProvider
}

// Init registers the per-frame ticker; Tick streams the provider forward and
// repaints. This is the "data streaming into a live UI" path — on device it is a
// platform callback instead of a synthetic Advance.
func (s *healthState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

func (s *healthState) Tick(dt float64) bool {
	s.SetState(func() { s.p.Advance(dt) })
	return true
}

func (s *healthState) Build(ctx widget.Ctx) widget.Widget {
	children := []widget.Widget{s.header()}
	for _, sp := range specs {
		children = append(children, s.card(sp))
	}
	return widget.Fill{Color: bg, Child: widget.Scroll{
		Child: widget.Padding{
			Insets: geom.InsetsSymmetric(18, 22),
			Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: children},
		},
	}}
}

func (s *healthState) header() widget.Widget {
	return widget.Padding{
		Insets: geom.Insets{Bottom: 18},
		Child: widget.Flex{CrossAlign: layout.CrossStart, Children: []widget.Widget{
			widget.Text{S: "Health", Size: 32, Color: ink},
			widget.Text{S: s.p.Name(), Size: 14, Color: sub},
		}},
	}
}

func (s *healthState) card(sp spec) widget.Widget {
	val, _ := s.p.Latest(sp.m)
	series := s.p.Series(sp.m)

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

	return widget.Padding{
		Insets: geom.Insets{Bottom: 14},
		Child: widget.Sized{H: 170, Child: widget.Decorated{Color: card, Radius: 18, Child: widget.Padding{
			All:   16,
			Child: widget.Flex{CrossAlign: layout.CrossStretch, Children: []widget.Widget{title, value, chart}},
		}}},
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
	c.FillPath(area, accent.WithAlpha(0.12))

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

func main() {
	if err := app.Run(Health{}, app.Config{
		Title:      "Health",
		Size:       geom.Size{W: 390, H: 760}, // phone-portrait, signalling the mobile target
		Background: bg,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
