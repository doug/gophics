package theme

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Tappable is a themed tap target with press feedback for arbitrary content —
// list rows, cells, selectable cards. It shows a press highlight (a translucent
// tint behind the child that flashes on pointer-down and eases out on release),
// giving touch the feedback hover can't, and fires OnTap on release over it.
// It's the row-level counterpart to Button: wrap a row's content in one instead
// of a bare widget.Interactive so taps feel native.
type Tappable struct {
	Child widget.Widget
	OnTap func()
	// OnLongPress, if set, fires on a press-and-hold and plays a medium haptic
	// (the universally tactile gesture on mobile).
	OnLongPress func()
	// Background is the row's resting fill. The press/hover highlight darkens
	// from it, so at rest the row shows exactly this color. Zero (transparent)
	// means the row shows no fill at rest and the highlight is a translucent
	// tint over whatever is behind it (for rows on a colored list surface).
	Background paint.Color
	// Radius is the corner radius of the fill/highlight; 0 is square, right for
	// full-bleed list rows. Set it to match an inset/rounded card.
	Radius float32
	// Pad insets the child within the fill, so the highlight extends to a
	// comfortable row height around tight content. Zero adds none.
	Pad geom.Insets
	// Haptic, when set, plays a light selection tick on tap. Off by default —
	// list rows don't buzz on every tap natively; enable it for a deliberate
	// pick (choosing an item that commits an action).
	Haptic bool
}

func (t Tappable) CreateState() widget.State { return &tappableState{} }

type tappableState struct {
	widget.StateBase[Tappable]
	ctx     widget.Ctx
	hovered bool
	press   *anim.Controller // highlight: full on down, eased fade on release
}

func (s *tappableState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.press = &anim.Controller{
		Duration: 180 * time.Millisecond, Curve: anim.EaseOut,
		OnChange: func() { s.SetState(func() {}) },
	}
	ctx.AddTicker(s.press)
}

func (s *tappableState) pressIn() { s.press.Jump(1) }

func (s *tappableState) pressOut() {
	if s.ctx.ReduceMotion() {
		s.press.Jump(0)
		return
	}
	s.press.Reverse()
	s.ctx.Invalidate()
}

func (s *tappableState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	t := s.W()
	// amt is the highlight strength: a whisper on hover (desktop), a firmer
	// press tint scaled by the release animation (the touch-native feedback).
	amt := float32(0)
	if s.hovered {
		amt = 0.05
	}
	if p := s.press.Value(); p > 0 {
		amt = 0.05 + 0.09*p
	}
	// With an opaque resting Background, darken it toward the text color; with a
	// transparent one, paint a translucent tint over whatever's behind the row.
	// Both fill exactly the child's bounds (Decorated sizes to its child).
	var fill paint.Color
	if t.Background.A > 0 {
		fill = paint.Lerp(t.Background, th.Text, amt)
	} else {
		fill = th.Text
		fill.A = amt
	}

	child := t.Child
	if t.Pad != (geom.Insets{}) {
		child = widget.Padding{Insets: t.Pad, Child: child}
	}
	h := widget.Handler{
		OnTap:      s.onTap,
		OnEnter:    func() { s.SetState(func() { s.hovered = true }) },
		OnExit:     func() { s.SetState(func() { s.hovered = false }) },
		OnPress:    func(geom.Pt) { s.pressIn() },
		OnPressEnd: s.pressOut,
	}
	if t.OnLongPress != nil {
		h.OnLongPress = s.onLongPress
	}
	return widget.Interactive{
		Handler: h,
		Child:   widget.Decorated{Color: fill, Radius: t.Radius, Child: child},
	}
}

func (s *tappableState) onTap() {
	t := s.W()
	if t.OnTap == nil {
		return
	}
	if t.Haptic {
		haptic(s.ctx, shell.HapticSelection)
	}
	t.OnTap()
}

func (s *tappableState) onLongPress() {
	haptic(s.ctx, shell.HapticMedium)
	s.W().OnLongPress()
}
