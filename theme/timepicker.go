package theme

import (
	"fmt"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// TimePicker is a minimal hour/minute chooser: two stepper columns (an up and
// down chevron around a two-digit value) that wrap at their bounds. Every
// change reports the new time via OnPick.
type TimePicker struct {
	Hour, Min int // initial values (Hour 0-23, Min 0-59)
	OnPick    func(hour, min int)
}

func (t TimePicker) CreateState() widget.State { return &timePickerState{} }

type timePickerState struct {
	widget.StateBase[TimePicker]
	hour, min int
}

func (s *timePickerState) Init(widget.Ctx) {
	s.hour = wrap(s.W().Hour, 24)
	s.min = wrap(s.W().Min, 60)
}

// step adjusts the hour (unit true) or minute by delta, wrapping, and reports.
func (s *timePickerState) step(hour bool, delta int) {
	s.SetState(func() {
		if hour {
			s.hour = wrap(s.hour+delta, 24)
		} else {
			s.min = wrap(s.min+delta, 60)
		}
	})
	if f := s.W().OnPick; f != nil {
		f(s.hour, s.min)
	}
}

func (s *timePickerState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	colon := widget.Text{S: ":", Font: FontBold, Size: th.Type.Display, Color: th.Muted}
	return widget.Row(
		s.column(th, true, s.hour),
		widget.Padding{Insets: geom.InsetsSymmetric(6, 0), Child: colon},
		s.column(th, false, s.min),
	)
}

// column is one stepper: up chevron, value, down chevron.
func (s *timePickerState) column(th Theme, hour bool, val int) widget.Widget {
	value := widget.Text{S: fmt.Sprintf("%02d", val), Font: FontBold, Size: th.Type.Display, Color: th.Text}
	c := widget.Column(
		vArrow(th, true, func() { s.step(hour, +1) }),
		widget.Sized{H: 4},
		value,
		widget.Sized{H: 4},
		vArrow(th, false, func() { s.step(hour, -1) }),
	)
	c.CrossAlign = 1 // CrossCenter
	return c
}

// vArrow is a tappable up/down chevron.
func vArrow(th Theme, up bool, onTap func()) widget.Widget {
	return Tappable{
		Radius: 8,
		Haptic: true,
		OnTap:  onTap,
		Child: widget.Padding{All: 6, Child: widget.Canvas{W: 28, H: 20, Draw: func(c paint.Canvas, size geom.Size) {
			cx := size.W / 2
			tipY, baseY := float32(13), float32(7)
			if up {
				tipY, baseY = 7, 13
			}
			c.Line(geom.Pt{X: cx - 6, Y: baseY}, geom.Pt{X: cx, Y: tipY}, 2, th.Muted)
			c.Line(geom.Pt{X: cx, Y: tipY}, geom.Pt{X: cx + 6, Y: baseY}, 2, th.Muted)
		}}},
	}
}

// ShowTimePicker presents a TimePicker as a dialog; onPick fires on each change.
func ShowTimePicker(ctx widget.Ctx, hour, min int, onPick func(hour, min int)) (dismiss func()) {
	return ShowDialog(ctx, TimePicker{Hour: hour, Min: min, OnPick: onPick})
}

// wrap normalizes v into [0, n).
func wrap(v, n int) int { return ((v % n) + n) % n }
