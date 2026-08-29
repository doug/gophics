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
	// origin is the control's top-left in global coordinates, derived at press
	// from the pointer's global and local positions. The popup hangs from the
	// control, not from wherever inside it the user happened to press.
	origin  geom.Pt
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
	th := Of(ctx)
	haptic(ctx, shell.HapticSelection)
	w := s.width
	if w < 120 {
		w = 120
	}
	s.SetState(func() { s.open = true })
	// Hang the list directly under the control. The seam is deliberately
	// narrow — the list is part of this control, not a card that happens to
	// be nearby — and showSelect gives it the control's own open-state border
	// so the two read as one object.
	below := geom.Pt{X: s.origin.X, Y: s.origin.Y + dropdownHeight + dropdownSeam}
	s.dismiss = showSelect(ctx, below, w, th.Primary, openBorderWidth, d.Options, d.Selected, func(i int) {
		if f := s.W().OnChange; f != nil {
			f(i)
		}
	}, func() { // onClose: the popup dismissed for any reason
		s.dismiss = nil
		s.SetState(func() { s.open = false })
	})
}

// dropdownHeight is the control's fixed height; the popup hangs below it.
const dropdownHeight float32 = 40

// openBorderWidth is the accent border the control wears while its list is
// open, and that the list wears to match.
const openBorderWidth float32 = 1.5

// dropdownSeam is the gap between the control and its open list. Small enough
// that they read as one control, wide enough that the two rounded edges do not
// pinch into each other.
const dropdownSeam float32 = 2

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
		border, bw = th.Primary, openBorderWidth
	}
	bg := th.Surface
	if s.hovered {
		bg = paint.Lerp(bg, th.Text, 0.05)
	}

	// The chrome is a full-width Canvas (fills its container like Slider) that
	// paints the surface, border, and chevron and records its width, so the
	// popup can match it. The label is layered on top, left-aligned and centered.
	surface := widget.Canvas{H: dropdownHeight, Draw: func(c paint.Canvas, size geom.Size) {
		s.width = size.W
		r := geom.Rect{Max: size.Pt()}
		c.FillRRect(r, th.Radius, bg)
		c.StrokeRRect(r, th.Radius, bw, border)
		// Chevron near the right edge, vertically centered.
		cx, cy := size.W-12, size.H/2
		drawChevron(c, geom.Pt{X: cx, Y: cy}, s.open, th.Muted)
	}}
	// Sized to the control's own height on purpose. Without it this layer
	// shrink-wraps the text, so Align has only a text-tall box to centre
	// within and the Stack pins that box to the top — the label rode high
	// while the chevron, drawn at size.H/2 by the Canvas, sat correctly
	// centred. Matching the Canvas height gives both the same axis.
	labelLayer := widget.Sized{H: dropdownHeight, Child: widget.Padding{
		Insets: geom.Insets{Left: labelInset, Right: 34},
		Child:  widget.Align{X: 0, Y: 0.5, Child: widget.Text{S: label, Size: th.Type.Body, Color: labelColor, MaxLines: 1, Ellipsis: true}},
	}}

	return widget.Interactive{
		Gestures: widget.Gestures{
			// The popup must hang from the control's own edge. A press reports
			// its position in the control's local coordinates, and Input reports
			// the same point globally, so the difference is the control's origin
			// — no global-rect API needed. Anchoring at the raw pointer instead
			// (as this did) opened the list wherever the finger happened to land,
			// so it appeared in a different place on every tap.
			OnPress: func(local geom.Pt) {
				p := ctx.Input().Pointer()
				s.origin = geom.Pt{X: p.X - local.X, Y: p.Y - local.Y}
			},
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
func showSelect(ctx widget.Ctx, topLeft geom.Pt, width float32, border paint.Color, borderWidth float32, options []string, selected int, onPick func(int), onClose func()) (dismiss func()) {
	ov := ctx.MustOf[widget.Overlay]()
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
			// Rounded and inset (see the menu's padding below) so the
			// selected row's fill nests inside the card's own corners
			// instead of squaring off across them.
			Radius: rowRadius(th),
			// menuInset + this = the control's own 12pt label inset, so the
			// selected value does not shift sideways when the list opens.
			Pad:   geom.InsetsSymmetric(labelInset-menuInset, 10),
			Child: row,
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

	// Same border as the control wears while open, at the same width: a faint
	// decorative hairline here made the list look like an unrelated card that
	// had landed underneath.
	menu := widget.Decorated{
		Color: th.Elevated, Radius: th.Radius, BorderColor: border, BorderWidth: borderWidth,
		Child: widget.Sized{W: width, Child: widget.Padding{All: menuInset, Child: col}},
	}
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: modalScrim{
		OnDismiss: closeFn,
		// No dim: the control stays lit so it and its list read as one thing.
		Clear: true,
		Child: widget.Padding{
			Insets: geom.Insets{Left: topLeft.X, Top: topLeft.Y},
			Child:  widget.Align{X: 0, Y: 0, Child: menu},
		},
	}})
	return closeFn
}

// labelInset is how far the control's own label sits from its left edge; the
// list's rows line up with it.
const labelInset float32 = 12

// menuInset is the gap between the list's border and its rows, so a row
// highlight never has to be clipped against the card's rounded corners.
const menuInset float32 = 4

// rowRadius keeps a row's highlight concentric with the card around it.
func rowRadius(th Theme) float32 {
	if r := th.Radius - menuInset; r > 0 {
		return r
	}
	return 0
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
