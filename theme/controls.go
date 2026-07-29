package theme

import (
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// Switch is an animated on/off toggle. Controlled: On is the source of
// truth, OnChange reports the requested new value.
type Switch struct {
	On       bool
	OnChange func(bool)
}

func (s Switch) CreateState() widget.State { return &switchState{} }

type switchState struct {
	widget.StateBase[Switch]
	ctx  widget.Ctx
	knob *anim.Controller
}

func (s *switchState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.knob = &anim.Controller{Duration: 140 * time.Millisecond, OnChange: func() { s.SetState(nil) }}
	ctx.AddTicker(s.knob)
	if s.W().On {
		s.knob.Jump(1)
	}
}

func (s *switchState) Dispose() { s.ctx.RemoveTicker(s.knob) }

func (s *switchState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	on := s.W().On
	if on && !s.knob.Running() && s.knob.Value() < 1 {
		s.knob.Forward()
	} else if !on && !s.knob.Running() && s.knob.Value() > 0 {
		s.knob.Reverse()
	}
	const w, h = 44, 26
	t := s.knob.Value()
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() {
			if f := s.W().OnChange; f != nil {
				f(!on)
			}
		}},
		Child: widget.Canvas{W: w, H: h, Draw: func(c paint.Canvas, size geom.Size) { r := geom.Rect{Max: size.Pt()};
			track := paint.Lerp(th.Border, th.Primary, t)
			c.FillRRect(r, h/2, track)
			cx := r.Min.X + h/2 + t*(w-h)
			cy := r.Min.Y + h/2
			c.FillRRect(geom.RectXYWH(cx-9, cy-9, 18, 18), 9, th.OnPrimary)
		}},
	}
}

// Checkbox is a labeled boolean box. Controlled via Checked/OnChange.
type Checkbox struct {
	Checked bool
	Label   string
	OnChange func(bool)
}

func (cb Checkbox) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	box := widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) { r := geom.Rect{Max: size.Pt()};
		if cb.Checked {
			c.FillRRect(r, 5, th.Primary)
			c.Line(r.Min.Add(geom.Pt{X: 4, Y: 10}), r.Min.Add(geom.Pt{X: 8, Y: 14}), 2, th.OnPrimary)
			c.Line(r.Min.Add(geom.Pt{X: 8, Y: 14}), r.Min.Add(geom.Pt{X: 16, Y: 5}), 2, th.OnPrimary)
		} else {
			c.StrokeRRect(r, 5, 1.5, th.Border)
		}
	}}
	var child widget.Widget = box
	if cb.Label != "" {
		row := widget.Row(box, widget.Sized{W: 8}, widget.Text{S: cb.Label, Size: 14, Color: th.Text})
		child = row
	}
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() {
			if cb.OnChange != nil {
				cb.OnChange(!cb.Checked)
			}
		}},
		Child: child,
	}
}

// Slider selects a value in [0,1] by dragging. It fills its available
// width. Controlled via Value/OnChange.
type Slider struct {
	Value    float32
	OnChange func(float32)
}

func (s Slider) CreateState() widget.State { return &sliderState{} }

type sliderState struct {
	widget.StateBase[Slider]
	width float32 // captured from the last paint, used to map drag → value
}

func (s *sliderState) set(x float32) {
	if s.width <= 0 || s.W().OnChange == nil {
		return
	}
	v := x / s.width
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.W().OnChange(v)
}

func (s *sliderState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	val := s.W().Value
	return widget.Interactive{
		Handler: widget.Handler{
			OnPress: func(p geom.Pt) { s.set(p.X) },
			OnDrag:  func(p, _ geom.Pt) { s.set(p.X) },
		},
		Child: widget.Canvas{H: 28, Draw: func(c paint.Canvas, size geom.Size) { r := geom.Rect{Max: size.Pt()};
			s.width = r.Dx()
			cy := r.Min.Y + r.Dy()/2
			c.FillRRect(geom.RectXYWH(r.Min.X, cy-2, r.Dx(), 4), 2, th.Border)
			fillW := val * r.Dx()
			c.FillRRect(geom.RectXYWH(r.Min.X, cy-2, fillW, 4), 2, th.Primary)
			cx := r.Min.X + fillW
			c.FillRRect(geom.RectXYWH(cx-8, cy-8, 16, 16), 8, th.Primary)
		}},
	}
}

// Radio is one option in a single-select group. Selected shows a filled
// dot; tapping requests selection via OnSelect.
type Radio struct {
	Selected bool
	Label    string
	OnSelect func()
}

func (rd Radio) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	dot := widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) { r := geom.Rect{Max: size.Pt()};
		ring := th.Border
		if rd.Selected {
			ring = th.Primary
		}
		c.StrokeRRect(r, 10, 1.5, ring)
		if rd.Selected {
			c.FillRRect(geom.RectXYWH(r.Min.X+5, r.Min.Y+5, 10, 10), 5, th.Primary)
		}
	}}
	var child widget.Widget = dot
	if rd.Label != "" {
		child = widget.Row(dot, widget.Sized{W: 8}, widget.Text{S: rd.Label, Size: 14, Color: th.Text})
	}
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() {
			if rd.OnSelect != nil {
				rd.OnSelect()
			}
		}},
		Child: child,
	}
}
