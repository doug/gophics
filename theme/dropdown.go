package theme

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Dropdown is a themed select: a bordered surface button showing the current
// selection (or a placeholder) and a chevron. Tapping opens an anchored popup
// listing the options with the current one highlighted; picking one fires
// OnChange and closes the popup.
//
// Controlled: Selected is the source of truth — an index into Options, or any
// out-of-range value (e.g. -1) to show the Placeholder — and OnChange reports
// the requested new index (matching Tabs and Segmented). Requires an
// OverlayHost in scope for the popup (app.NewCore installs one). Themed via
// theme.Of.
type Dropdown struct {
	Options     []string
	Selected    int
	Placeholder string
	OnChange    func(int)
}

func (d Dropdown) CreateState() widget.State { return &dropdownState{} }

type dropdownState struct {
	widget.StateBase[Dropdown]
	ctx     widget.Ctx
	hovered bool
	open    bool
	anchor  geom.Pt // global pointer at press, where the popup is anchored
	width   float32 // last painted button width, so the popup matches it
	dismiss func()  // closes the open popup, nil when closed
}

func (s *dropdownState) Init(ctx widget.Ctx) { s.ctx = ctx }

// Dispose closes a still-open popup so it can't outlive the control.
func (s *dropdownState) Dispose() {
	if s.dismiss != nil {
		s.dismiss()
	}
}

func (s *dropdownState) toggle(ctx widget.Ctx) {
	if s.open {
		if s.dismiss != nil {
			s.dismiss()
		}
		return
	}
	d := s.W()
	haptic(ctx, shell.HapticSelection)
	w := s.width
	if w < 120 {
		w = 120
	}
	s.SetState(func() { s.open = true })
	s.dismiss = showSelect(ctx, s.anchor, w, d.Options, d.Selected, func(i int) {
		if f := s.W().OnChange; f != nil {
			f(i)
		}
	}, func() { // onClose: the popup dismissed for any reason
		s.dismiss = nil
		s.SetState(func() { s.open = false })
	})
}

func (s *dropdownState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	d := s.W()

	label, labelColor := d.Placeholder, th.Muted
	if d.Selected >= 0 && d.Selected < len(d.Options) {
		label, labelColor = d.Options[d.Selected], th.Text
	}

	// The border accents (like a focused Field) while the popup is open.
	border, bw := th.Outline, float32(1)
	if s.open {
		border, bw = th.Primary, 1.5
	}
	bg := th.Surface
	if s.hovered {
		bg = paint.Lerp(bg, th.Text, 0.05)
	}

	// The chrome is a full-width Canvas (fills its container like Slider) that
	// paints the surface, border, and chevron and records its width, so the
	// popup can match it. The label is layered on top, left-aligned and centered.
	const h float32 = 40
	surface := widget.Canvas{H: h, Draw: func(c paint.Canvas, size geom.Size) {
		s.width = size.W
		r := geom.Rect{Max: size.Pt()}
		c.FillRRect(r, th.Radius, bg)
		c.StrokeRRect(r, th.Radius, bw, border)
		// Chevron near the right edge, vertically centered.
		cx, cy := size.W-12, size.H/2
		drawChevron(c, geom.Pt{X: cx, Y: cy}, s.open, th.Muted)
	}}
	labelLayer := widget.Padding{
		Insets: geom.Insets{Left: 12, Right: 34},
		Child:  widget.Align{X: 0, Y: 0.5, Child: widget.Text{S: label, Size: th.Type.Body, Color: labelColor, MaxLines: 1, Ellipsis: true}},
	}

	return widget.Interactive{
		Handler: widget.Handler{
			// Capture the global pointer at press so the popup anchors under the
			// control (the widget tree exposes no global rect; the pointer is the
			// reliable proxy, matching ShowMenu's point-anchored API).
			OnPress: func(geom.Pt) { s.anchor = ctx.Input().Pointer() },
			OnTap:   func() { s.toggle(ctx) },
			OnEnter: func() { s.SetState(func() { s.hovered = true }) },
			OnExit:  func() { s.SetState(func() { s.hovered = false }) },
		},
		Child: widget.Stack{Children: []widget.Widget{surface, labelLayer}},
	}
}

// showSelect presents options in a card anchored at topLeft (logical coords),
// above the app, reusing the Overlay mechanism ShowMenu uses. The row at
// `selected` is highlighted. Picking a row calls onPick(i) then closes; tapping
// outside or Escape closes. onClose fires whenever the popup dismisses (a pick
// or an outside tap), so the caller can drop its open state.
func showSelect(ctx widget.Ctx, topLeft geom.Pt, width float32, options []string, selected int, onPick func(int), onClose func()) (dismiss func()) {
	ov := widget.MustOf[widget.Overlay](ctx)
	th := Of(ctx)
	var tok widget.OverlayToken
	closed := false
	closeFn := func() {
		if closed {
			return
		}
		closed = true
		tok.Dismiss()
		if onClose != nil {
			onClose()
		}
	}

	rows := make([]widget.Widget, len(options))
	for i, opt := range options {
		on := i == selected
		textColor := th.Text
		var bg paint.Color
		if on {
			textColor = th.Primary
			bg = th.Selection
		}
		trailing := widget.Widget(widget.Sized{W: 16})
		if on {
			trailing = check{color: th.Primary}
		}
		row := widget.Widget(widget.Row(
			widget.Text{S: opt, Size: th.Type.Body, Color: textColor},
			widget.Spacer(),
			widget.Sized{W: 10},
			trailing,
		))
		rows[i] = Tappable{
			Background: bg,
			Pad:        geom.InsetsSymmetric(14, 10),
			Child:      row,
			OnTap: func() {
				closeFn()
				if onPick != nil {
					onPick(i)
				}
			},
		}
	}
	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch

	menu := widget.Decorated{
		Color: th.Surface, Radius: th.Radius, BorderColor: th.Border, BorderWidth: 1,
		Child: widget.Sized{W: width, Child: col},
	}
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: modalScrim{
		OnDismiss: closeFn,
		Child: widget.Padding{
			Insets: geom.Insets{Left: topLeft.X, Top: topLeft.Y},
			Child:  widget.Align{X: 0, Y: 0, Child: menu},
		},
	}})
	return closeFn
}

// drawChevron paints a small down- (or up-) pointing arrow centered at c, the
// affordance that this surface opens a list.
func drawChevron(canvas paint.Canvas, center geom.Pt, up bool, col paint.Color) {
	const half, drop = 5.0, 3.0 // half-width and vertical reach of the "v"
	var l, mid, r geom.Pt
	if up { // apex at the top
		l = geom.Pt{X: center.X - half, Y: center.Y + drop/2}
		mid = geom.Pt{X: center.X, Y: center.Y - drop/2}
		r = geom.Pt{X: center.X + half, Y: center.Y + drop/2}
	} else { // apex at the bottom
		l = geom.Pt{X: center.X - half, Y: center.Y - drop/2}
		mid = geom.Pt{X: center.X, Y: center.Y + drop/2}
		r = geom.Pt{X: center.X + half, Y: center.Y - drop/2}
	}
	canvas.Line(l, mid, 1.5, col)
	canvas.Line(mid, r, 1.5, col)
}

// check draws a small checkmark marking the selected option.
type check struct{ color paint.Color }

func (ck check) Build(widget.Ctx) widget.Widget {
	return widget.Canvas{W: 16, H: 16, Draw: func(c paint.Canvas, size geom.Size) {
		c.Line(geom.Pt{X: 3, Y: 8}, geom.Pt{X: 6, Y: 12}, 2, ck.color)
		c.Line(geom.Pt{X: 6, Y: 12}, geom.Pt{X: 13, Y: 4}, 2, ck.color)
	}}
}
