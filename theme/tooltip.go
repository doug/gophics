package theme

import (
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// Tooltip shows a short explanatory label near its child after the pointer
// rests on it, and on a long press where there is no pointer.
//
// It deliberately does not replace an accessible name: the tip is also
// published as the child's semantic hint, so the same words reach a screen
// reader without depending on a hover a touch user cannot perform.
type Tooltip struct {
	// Message is the tip text. Empty disables the tooltip entirely.
	Message string
	// Delay is how long the pointer must rest before the tip appears.
	// 0 → 500ms, which is long enough not to fire while crossing the control.
	Delay time.Duration
	Child widget.Widget
}

func (t Tooltip) CreateState() widget.State { return &tooltipState{} }

type tooltipState struct {
	widget.StateBase[Tooltip]
	ctx widget.Ctx
	// hover counts down to zero while the pointer rests; it is a ticker
	// rather than a timer so it lives on the frame loop and stops the moment
	// the widget is disposed.
	hover  hoverTimer
	tok    widget.OverlayToken
	shown  bool
	origin geom.Pt
}

func (s *tooltipState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.hover.fire = func() { s.show() }
	ctx.AddTicker(&s.hover)
}

func (s *tooltipState) Dispose() {
	s.hide()
	s.ctx.RemoveTicker(&s.hover)
}

func (s *tooltipState) delay() float32 {
	if d := s.W().Delay; d > 0 {
		return float32(d.Seconds())
	}
	return 0.5
}

func (s *tooltipState) show() {
	if s.shown || s.W().Message == "" {
		return
	}
	s.shown = true
	th := Of(s.ctx)
	ov := widget.MustOf[widget.Overlay](s.ctx)

	// The tip hangs below-right of the pointer, the position that keeps it
	// clear of the cursor and of the control being described.
	at := s.origin.Add(geom.Pt{X: 12, Y: 20})
	card := widget.Decorated{
		Color: th.Elevated, Radius: 6, BorderColor: th.Border, BorderWidth: 1, Blur: th.Blur,
		Child: widget.Padding{
			Insets: geom.Insets{Left: 8, Right: 8, Top: 4, Bottom: 4},
			Child: widget.Text{
				S: s.W().Message, Size: th.Type.Caption, Color: th.Text, MaxLines: 2,
			},
		},
	}
	// Positioned via a Stack aligned to the top-left, offset by padding: the
	// overlay fills the window, so padding is how a child is placed in it.
	s.tok = ov.Show(widget.Padding{
		Insets: geom.Insets{Left: at.X, Top: at.Y},
		Child:  widget.Align{X: 0, Y: 0, Child: card},
	})
}

func (s *tooltipState) hide() {
	s.hover.stop()
	if s.shown {
		s.tok.Dismiss()
		s.shown = false
	}
}

func (s *tooltipState) Build(ctx widget.Ctx) widget.Widget {
	w := s.W()
	if w.Message == "" {
		return w.Child
	}
	return widget.Interactive{
		Handler: widget.Handler{
			OnEnter: func() {
				s.origin = ctx.Input().Pointer()
				s.hover.start(s.delay())
			},
			OnExit: func() { s.hide() },
			// Touch has no hover, so the tip is reachable by holding — the
			// same gesture the platform conventions use for "what is this".
			OnLongPress: func() {
				s.origin = ctx.Input().Pointer()
				s.show()
			},
			OnPressEnd: func() { s.hide() },
		},
		// The message doubles as the semantic hint so it is available without
		// hovering at all.
		Child: widget.Semantics{Hint: w.Message, Child: w.Child},
	}
}

// hoverTimer counts down and fires once. It reports "still running" only while
// counting, so a resting UI costs no frames.
type hoverTimer struct {
	left    float32
	running bool
	fire    func()
}

func (t *hoverTimer) start(seconds float32) {
	t.left, t.running = seconds, true
}

func (t *hoverTimer) stop() { t.running = false }

func (t *hoverTimer) Tick(dt float64) bool {
	if !t.running {
		return false
	}
	t.left -= float32(dt)
	if t.left <= 0 {
		t.running = false
		if t.fire != nil {
			t.fire()
		}
		return false
	}
	return true
}
