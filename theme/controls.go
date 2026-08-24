package theme

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Switch is an animated on/off toggle. Controlled: On is the source of
// truth, OnChange reports the requested new value.
type Switch struct {
	On       bool
	OnChange func(bool)
	// Label names what the switch controls, for assistive technology. A
	// switch is drawn, not written, so without this a screen reader can only
	// say "on" — true, and useless.
	Label string
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
	track := widget.Canvas{W: w, H: h, Draw: func(c paint.Canvas, size geom.Size) {
		r := geom.Rect{Max: size.Pt()}
		col := paint.Lerp(th.Outline, th.Primary, t)
		c.FillRRect(r, h/2, col)
		cx := r.Min.X + h/2 + t*(w-h)
		cy := r.Min.Y + h/2
		c.FillRRect(geom.RectXYWH(cx-9, cy-9, 18, 18), 9, th.OnPrimary)
	}}
	checked := on
	return widget.Interactive{
		Sem: &layout.SemInfo{Role: layout.RoleSwitch, Label: s.W().Label, Checked: &checked},
		Handler: widget.Handler{OnTap: func() {
			if f := s.W().OnChange; f != nil {
				haptic(ctx, shell.HapticSelection) // a light tick as the value flips
				f(!on)
			}
		}},
		// The tap area is the full minimum target, not the 26pt-tall track: a
		// switch drawn to look slim still has to be as easy to hit as anything
		// else, and a target shorter than a fingertip is the difference between
		// "toggles when I tap it" and "sometimes toggles".
		Child: touchTarget(track),
	}
}

// MinTouchTarget is the smallest comfortable tap area in logical pixels. Both
// Apple and Google land on 44–48; a control drawn smaller than this keeps its
// looks and grows its *hit* area to meet it.
const MinTouchTarget = 44

// touchTarget centres a control inside at least MinTouchTarget in each axis, so
// the surrounding Interactive covers a finger-sized area. It adds layout space,
// which is the honest cost: the alternative is a control that silently misses.
func touchTarget(child widget.Widget) widget.Widget {
	return widget.Sized{W: MinTouchTarget, H: MinTouchTarget, Child: widget.Center(child)}
}

// touchTargetH raises a control to at least MinTouchTarget tall without
// constraining its width.
//
// A labeled checkbox or radio is already wide enough to hit; it is the height
// that falls short, at the 20pt of the box itself. touchTarget pins both axes,
// which suits a bare switch track but would clip a label, so rows use this.
//
// Centred vertically and aligned to the leading edge horizontally, not
// centred on both. Centre used to do both, which the comment above already
// said it should not: given a column wider than the control — which is every
// form — a checkbox drifted to the middle of the row while its label sat
// beside it, and a stack of them came out ragged instead of aligned down
// their boxes. Directional, so it follows the leading edge in RTL rather
// than pinning to the left.
func touchTargetH(child widget.Widget) widget.Widget {
	return widget.Sized{H: MinTouchTarget, Child: widget.Align{
		X: 0, Y: 0.5, Directional: true, Child: child,
	}}
}

// Checkbox is a labeled boolean box. Controlled via Checked/OnChange.
type Checkbox struct {
	Checked  bool
	Label    string
	OnChange func(bool)
}

func (cb Checkbox) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	box := widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) {
		r := geom.Rect{Max: size.Pt()}
		if cb.Checked {
			c.FillRRect(r, 5, th.Primary)
			c.Line(r.Min.Add(geom.Pt{X: 4, Y: 10}), r.Min.Add(geom.Pt{X: 8, Y: 14}), 2, th.OnPrimary)
			c.Line(r.Min.Add(geom.Pt{X: 8, Y: 14}), r.Min.Add(geom.Pt{X: 16, Y: 5}), 2, th.OnPrimary)
		} else {
			c.StrokeRRect(r, 5, 1.5, th.Outline)
		}
	}}
	var child widget.Widget = box
	if cb.Label != "" {
		row := widget.Row(box, widget.Sized{W: 8}, widget.Text{S: cb.Label, Size: 14, Color: th.Text})
		child = row
	}
	checked := cb.Checked
	return widget.Interactive{
		Sem: &layout.SemInfo{Role: layout.RoleCheckbox, Label: cb.Label, Checked: &checked},
		Handler: widget.Handler{OnTap: func() {
			if cb.OnChange != nil {
				haptic(ctx, shell.HapticSelection)
				cb.OnChange(!cb.Checked)
			}
		}},
		Child: touchTargetH(child),
	}
}

// Slider selects a value in [0,1] by dragging. It fills its available
// width. Controlled via Value/OnChange.
type Slider struct {
	Value    float32
	OnChange func(float32)
	// Label names what the slider adjusts, for assistive technology.
	Label string
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
		Sem: &layout.SemInfo{
			Role: layout.RoleSlider, Label: s.W().Label, Value: formatPercent(val),
		},
		Handler: widget.Handler{
			OnPress: func(p geom.Pt) { s.set(p.X) },
			OnDrag:  func(p, _ geom.Pt) { s.set(p.X) },
		},
		Child: widget.Canvas{H: 28, Draw: func(c paint.Canvas, size geom.Size) {
			r := geom.Rect{Max: size.Pt()}
			s.width = r.Dx()
			cy := r.Min.Y + r.Dy()/2
			c.FillRRect(geom.RectXYWH(r.Min.X, cy-2, r.Dx(), 4), 2, th.Outline)
			fillW := val * r.Dx()
			c.FillRRect(geom.RectXYWH(r.Min.X, cy-2, fillW, 4), 2, th.Primary)
			cx := r.Min.X + fillW
			c.FillRRect(geom.RectXYWH(cx-8, cy-8, 16, 16), 8, th.Primary)
		}},
	}
}

// Radio is one option in a single-select group. Selected shows a filled
// dot; tapping requests selection via OnSelect.
//
// The callback is OnSelect, not OnChange, deliberately: a Radio is one item,
// not the group — it has no value that changes, it only reports "this option
// was chosen" (like Tappable.OnTap). The group's value lives with the caller,
// which sets Selected on each option.
type Radio struct {
	Selected bool
	Label    string
	OnSelect func()
}

func (rd Radio) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	dot := widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) {
		r := geom.Rect{Max: size.Pt()}
		ring := th.Outline
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
		Sem: &layout.SemInfo{
			Role: layout.RoleRadio, Label: rd.Label, Selected: rd.Selected,
		},
		Handler: widget.Handler{OnTap: func() {
			if rd.OnSelect != nil {
				haptic(ctx, shell.HapticSelection)
				rd.OnSelect()
			}
		}},
		Child: touchTargetH(child),
	}
}
