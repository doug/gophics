package theme

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Progress is a horizontal progress bar. It fills the width it is given.
//
// A negative Value means indeterminate: the bar sweeps a moving segment
// instead of filling, which is the honest rendering for work whose size is
// unknown. Determinate values are clamped to [0,1].
//
// Progress announces itself as a "progressbar" with its percentage, so a
// screen reader reports the same thing the eye reads.
type Progress struct {
	// Value in [0,1]; negative for indeterminate.
	Value float32
	// Height of the track. 0 → 4.
	Height float32
	// Color of the filled portion. Zero → the theme's Primary.
	Color paint.Color
	// Label names what is progressing, for assistive technology
	// ("Uploading photos"). Optional but strongly encouraged.
	Label string
}

func (p Progress) CreateState() widget.State { return &progressState{} }

type progressState struct {
	widget.StateBase[Progress]
	ctx  widget.Ctx
	tick sweepTicker
}

func (s *progressState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.tick.repaint = func() { s.SetState(nil) }
	s.tick.period = 1.4
	ctx.AddTicker(&s.tick)
}

func (s *progressState) Dispose() { s.ctx.RemoveTicker(&s.tick) }

func (s *progressState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	w := s.W()
	h := w.Height
	if h <= 0 {
		h = 4
	}
	fill := w.Color
	if fill == (paint.Color{}) {
		fill = th.Primary
	}
	indeterminate := w.Value < 0
	s.tick.running = indeterminate
	phase := s.tick.phase
	value := clamp01(w.Value)

	bar := widget.Canvas{H: h, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		r := geom.Rect{Max: size.Pt()}
		c.FillRRect(r, h/2, th.Outline.WithAlpha(0.28))
		if !indeterminate {
			if value > 0 {
				c.FillRRect(geom.RectXYWH(0, 0, size.W*value, h), h/2, fill)
			}
			return
		}
		// The sweeping segment runs off both ends so the motion reads as
		// continuous rather than as a bar bouncing between two walls.
		const seg = 0.35
		x := (phase*(1+seg) - seg) * size.W
		c.FillRRect(geom.RectXYWH(x, 0, size.W*seg, h), h/2, fill)
	}}

	return widget.Semantics{
		Role:  layout.RoleProgress,
		Label: progressLabel(w.Label, w.Value),
		Child: bar,
	}
}

// progressLabel builds the spoken description: the caller's label plus the
// percentage, or "busy" when there is no percentage to give.
func progressLabel(label string, value float32) string {
	pct := "busy"
	if value >= 0 {
		pct = formatPercent(clamp01(value))
	}
	if label == "" {
		return pct
	}
	return label + ", " + pct
}

// Spinner is an indeterminate circular activity indicator — the right choice
// when there is no measurable progress and no room for a bar.
type Spinner struct {
	// Size is the diameter. 0 → 20.
	Size float32
	// Color of the arc. Zero → the theme's Primary.
	Color paint.Color
	// Label names what is happening, for assistive technology.
	Label string
}

func (s Spinner) CreateState() widget.State { return &spinnerState{} }

type spinnerState struct {
	widget.StateBase[Spinner]
	ctx  widget.Ctx
	tick sweepTicker
	arc  paint.Path
}

func (s *spinnerState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.tick.repaint = func() { s.SetState(nil) }
	s.tick.period = 1.1
	s.tick.running = true
	ctx.AddTicker(&s.tick)
}

func (s *spinnerState) Dispose() { s.ctx.RemoveTicker(&s.tick) }

func (s *spinnerState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	w := s.W()
	d := w.Size
	if d <= 0 {
		d = 20
	}
	col := w.Color
	if col == (paint.Color{}) {
		col = th.Primary
	}
	phase := s.tick.phase
	stroke := d / 9
	if stroke < 1.5 {
		stroke = 1.5
	}

	dial := widget.Canvas{W: d, H: d, Draw: func(c paint.Canvas, size geom.Size) {
		cx, cy := size.W/2, size.H/2
		rad := (min(size.W, size.H) - stroke) / 2
		if rad <= 0 {
			return
		}
		// Sweep length breathes between a short and a long arc while the whole
		// thing rotates — one path, two rates, which is what stops it reading
		// as a rigidly spinning wheel.
		start := float64(phase) * 2 * math.Pi * 2
		sweep := float64(0.25+0.55*(1-cosNorm(phase*2))) * 2 * math.Pi
		// The path is retained on the state so scene diffing compares it by
		// identity; Reset keeps the same allocation across frames.
		s.arc.Reset()
		const steps = 32
		for i := 0; i <= steps; i++ {
			a := start + sweep*float64(i)/steps
			pt := geom.Pt{
				X: cx + rad*float32(math.Cos(a)),
				Y: cy + rad*float32(math.Sin(a)),
			}
			if i == 0 {
				s.arc.MoveTo(pt)
			} else {
				s.arc.LineTo(pt)
			}
		}
		c.StrokePath(&s.arc, stroke, col)
	}}

	label := w.Label
	if label == "" {
		label = "Busy"
	}
	return widget.Semantics{Role: layout.RoleProgress, Label: label, Child: dial}
}

// sweepTicker drives a 0→1 phase that wraps, for indeterminate animation. It
// reports "not running" when idle so a determinate Progress costs no frames.
type sweepTicker struct {
	phase   float32
	period  float32 // seconds per cycle
	running bool
	repaint func()
}

func (t *sweepTicker) Tick(dt float64) bool {
	if !t.running {
		return false
	}
	p := t.period
	if p <= 0 {
		p = 1
	}
	t.phase += float32(dt) / p
	for t.phase >= 1 {
		t.phase -= 1
	}
	if t.repaint != nil {
		t.repaint()
	}
	return true
}

// cosNorm maps a wrapping phase to a 0..1 cosine ease.
func cosNorm(phase float32) float32 {
	return float32(0.5 * (1 + math.Cos(float64(phase)*2*math.Pi)))
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// formatPercent renders a [0,1] fraction as a whole-number percentage without
// pulling in a formatting dependency for one string.
func formatPercent(v float32) string {
	n := int(v*100 + 0.5)
	if n <= 0 {
		return "0%"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:]) + "%"
}
