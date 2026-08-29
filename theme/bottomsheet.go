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

// ShowBottomSheet slides a rounded, full-width surface up from the bottom edge
// over a dimming scrim, above the whole app. Tapping the scrim, pressing
// Escape, or dragging the sheet down past a threshold dismisses it; the
// returned dismiss func closes it programmatically. The slide-up and the
// scrim fade are animated (respecting reduce-motion), and dismissal plays the
// exit animation before the overlay entry is removed. Requires an OverlayHost
// in scope (app.NewCore installs one).
func ShowBottomSheet(ctx widget.Ctx, content widget.Widget) (dismiss func()) {
	ov := ctx.MustOf[widget.Overlay]()
	th := Of(ctx)
	var tok widget.OverlayToken
	// The live sheet state animates entry/exit; the returned closer routes
	// through it (via handle.begin) so a programmatic dismiss plays the exit
	// too. handle.begin is populated when the sheet's state initializes.
	h := &sheetHandle{}
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
	// Overlay entries sit above the app's providers, so re-provide the theme
	// captured here for the themed widgets inside the sheet.
	sheet := bottomSheet{theme: th, content: content, handle: h,
		onGone: func() { tok.Dismiss() }}
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: sheet})
	return dismiss
}

// sheetHandle bridges the free ShowBottomSheet closer to the live sheet state.
type sheetHandle struct{ begin func() }

// bottomSheet is the stateful overlay body: it owns the slide/fade controller.
type bottomSheet struct {
	theme   Theme
	content widget.Widget
	handle  *sheetHandle
	onGone  func() // remove the overlay entry once the exit animation ends
}

func (b bottomSheet) CreateState() widget.State { return &bottomSheetState{} }

type bottomSheetState struct {
	widget.StateBase[bottomSheet]
	ctx     widget.Ctx
	t       *anim.Controller // 0 = hidden (below the fold, scrim clear), 1 = resting
	drag    float32          // live downward drag offset (px), added to the slide
	exiting bool
}

func (s *bottomSheetState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.t = &anim.Controller{
		Duration: 260 * time.Millisecond, Curve: anim.EaseOut,
		OnChange: func() {
			s.SetState(func() {})
			// Exit finished (settled at 0): drop the overlay entry.
			if s.exiting && !s.t.Running() && s.t.Value() == 0 {
				if cb := s.W().onGone; cb != nil {
					cb()
				}
			}
		},
	}
	ctx.AddTicker(s.t)
	s.W().handle.begin = s.close
	if ctx.ReduceMotion() {
		s.t.Jump(1) // appear immediately
	} else {
		s.t.Forward()
		ctx.Invalidate() // kick the frame loop so the entrance advances
	}
}

func (s *bottomSheetState) Dispose() { s.ctx.RemoveTicker(s.t) }

// close plays the exit animation, then onGone removes the entry.
func (s *bottomSheetState) close() {
	if s.exiting {
		return
	}
	s.exiting = true
	if s.ctx.ReduceMotion() {
		s.t.Jump(0) // OnChange sees exiting+settled → onGone
		return
	}
	s.t.Reverse()
	s.ctx.Invalidate()
}

func (s *bottomSheetState) Build(ctx widget.Ctx) widget.Widget {
	w := s.W()
	th := w.theme
	// LayoutBuilder gives the surface height, used as the slide distance so the
	// sheet starts fully below the fold regardless of its own content height.
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		v := s.t.Value()
		h := cs.Max.H
		if h <= 0 || h == layout.Inf {
			h = 600
		}
		width := cs.Max.W
		ty := (1-v)*h + s.drag

		scrim := widget.Interactive{
			Gestures: widget.Gestures{
				OnTap: s.close,
				OnKey: func(k shell.Key) {
					if k.Kind == shell.KeyPress && k.Code == shell.KeyEscape {
						s.close()
					}
				},
			},
			Child: widget.Fill{Color: paint.Color{A: 0.45 * v}},
		}

		bottomSafe := s.ctx.SafeInsets().Bottom
		surface := widget.Decorated{
			Color: th.Elevated, Radius: th.Radius, Blur: th.Blur,
			Child: widget.Padding{
				Insets: geom.Insets{Left: 20, Right: 20, Top: 12, Bottom: 20 + bottomSafe},
				Child: widget.Column(
					grabHandle(th),
					widget.Sized{H: 12},
					w.content,
				),
			},
		}
		// Drag-to-dismiss: the sheet follows a downward drag; a release past the
		// threshold closes, otherwise it springs back to rest.
		draggable := widget.Interactive{
			Gestures: widget.Gestures{
				DragAxis: widget.DragVertical,
				OnDrag: func(_, d geom.Pt) {
					s.SetState(func() { s.drag = maxF(0, s.drag+d.Y) })
				},
				OnRelease: func() {
					if s.drag > 100 {
						s.close()
					} else {
						s.SetState(func() { s.drag = 0 })
					}
				},
			},
			Child: widget.Sized{W: width, Child: surface},
		}

		sheet := widget.Align{X: 0.5, Y: 1,
			Child: widget.Transform{T: paint.Transform{TY: ty}, Child: draggable}}
		return widget.Stack{Children: []widget.Widget{scrim, sheet}}
	}}
}

// grabHandle is the small centered pill that signals a draggable sheet.
func grabHandle(th Theme) widget.Widget {
	return widget.Align{X: 0.5, Y: 0.5,
		Child: widget.Decorated{Color: th.Muted, Radius: 3,
			Child: widget.Sized{W: 36, H: 4}}}
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
