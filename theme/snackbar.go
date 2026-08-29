package theme

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// ShowSnackbar presents a transient toast near the bottom of the app with an
// optional action button. It fades and slides in, holds for a timeout, then
// fades and slides out and removes itself — the hold is driven by an
// anim.Controller acting as a timer (no goroutine sleeping on widget state).
// Tapping the action (or calling the returned dismiss) closes it early, still
// playing the exit. Unlike a dialog there is no scrim: the toast is
// non-modal and does not block input to the app. Requires an OverlayHost in
// scope (app.NewCore installs one).
//
//	theme.ShowSnackbar(ctx, "Saved")
//	theme.ShowSnackbar(ctx, "Deleted", theme.WithAction("Undo", undo),
//	    theme.WithDuration(6*time.Second))
func ShowSnackbar(ctx widget.Ctx, message string, opts ...SnackOption) (dismiss func()) {
	ov := ctx.MustOf[widget.Overlay]()
	th := Of(ctx)
	cfg := snackConfig{duration: 4 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	var tok widget.OverlayToken
	h := &snackHandle{}
	closed := false
	dismiss = func() {
		if closed {
			return
		}
		closed = true
		if h.begin != nil {
			h.begin()
		} else {
			tok.Dismiss()
		}
	}
	sb := snackbar{theme: th, message: message, cfg: cfg, handle: h,
		onGone: func() { tok.Dismiss() }}
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: sb})
	return dismiss
}

// SnackOption configures a snackbar.
type SnackOption func(*snackConfig)

type snackConfig struct {
	actionLabel string
	onAction    func()
	duration    time.Duration
}

// WithAction adds a trailing action button (e.g. "Undo"); tapping it runs
// onTap and dismisses the snackbar.
func WithAction(label string, onTap func()) SnackOption {
	return func(c *snackConfig) { c.actionLabel, c.onAction = label, onTap }
}

// WithDuration overrides how long the snackbar stays before auto-dismissing.
func WithDuration(d time.Duration) SnackOption {
	return func(c *snackConfig) { c.duration = d }
}

// snackHandle bridges the free ShowSnackbar closer to the live snackbar state.
type snackHandle struct{ begin func() }

// snackbar is the stateful overlay body: it owns the transition and hold
// (timeout) controllers.
type snackbar struct {
	theme   Theme
	message string
	cfg     snackConfig
	handle  *snackHandle
	onGone  func() // remove the overlay entry once the exit animation ends
}

func (b snackbar) CreateState() widget.State { return &snackbarState{} }

type snackbarState struct {
	widget.StateBase[snackbar]
	ctx     widget.Ctx
	t       *anim.Controller // 0 = hidden (below + faded), 1 = resting
	hold    *anim.Controller // a timer: completion triggers the exit
	exiting bool
}

func (s *snackbarState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.t = &anim.Controller{
		Duration: 220 * time.Millisecond, Curve: anim.EaseOut,
		OnChange: func() {
			s.SetState(func() {})
			if s.exiting && !s.t.Running() && s.t.Value() == 0 {
				if cb := s.W().onGone; cb != nil {
					cb()
				}
			}
		},
	}
	// hold runs 0→1 over the visible duration; on completion it starts the exit.
	// Using a controller (a per-frame ticker) keeps the delay on the UI
	// goroutine — no background sleep poking widget state.
	s.hold = &anim.Controller{
		Duration: s.W().cfg.duration, Curve: anim.Linear,
		OnChange: func() {
			if !s.exiting && !s.hold.Running() && s.hold.Value() >= 1 {
				s.close()
			}
		},
	}
	ctx.AddTicker(s.t)
	ctx.AddTicker(s.hold)
	s.W().handle.begin = s.close
	if ctx.ReduceMotion() {
		s.t.Jump(1)
	} else {
		s.t.Forward()
	}
	s.hold.Forward()
	ctx.Invalidate() // kick the frame loop so entrance + hold advance
}

func (s *snackbarState) Dispose() {
	s.ctx.RemoveTicker(s.t)
	s.ctx.RemoveTicker(s.hold)
}

// close plays the exit animation, then onGone removes the entry.
func (s *snackbarState) close() {
	if s.exiting {
		return
	}
	s.exiting = true
	if s.ctx.ReduceMotion() {
		s.t.Jump(0)
		return
	}
	s.t.Reverse()
	s.ctx.Invalidate()
}

func (s *snackbarState) Build(ctx widget.Ctx) widget.Widget {
	w := s.W()
	th := w.theme
	v := s.t.Value()
	// Inverse surface for the classic high-contrast toast look.
	bg, fg := th.Text, th.Bg

	row := []widget.Widget{
		widget.Text{S: w.message, Size: th.Type.Body, Color: fg},
	}
	if w.cfg.actionLabel != "" {
		row = append(row,
			widget.Sized{W: 16},
			widget.Spacer(),
			widget.Interactive{
				Gestures: widget.Gestures{OnTap: func() {
					if w.cfg.onAction != nil {
						w.cfg.onAction()
					}
					s.close()
				}},
				Child: widget.Text{S: w.cfg.actionLabel, Font: FontBold,
					Size: th.Type.Label, Color: th.Primary},
			},
		)
	}

	surface := widget.Decorated{
		Color: bg, Radius: th.Radius,
		Child: widget.Padding{
			Insets: geom.InsetsSymmetric(16, 12),
			Child:  widget.Row(row...),
		},
	}

	bottomSafe := s.ctx.SafeInsets().Bottom
	// Slide up a short distance and fade; positioned at the bottom, non-modal.
	toast := widget.Opacity{Alpha: v,
		Child: widget.Transform{T: paint.Transform{TY: (1 - v) * 24}, Child: surface}}
	return widget.Align{X: 0.5, Y: 1,
		Child: widget.Padding{
			Insets: geom.Insets{Left: 16, Right: 16, Bottom: 24 + bottomSafe, Top: 16},
			Child:  toast,
		}}
}
