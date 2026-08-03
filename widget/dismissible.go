package widget

import (
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// DismissDir constrains which way a Dismissible may be swiped.
type DismissDir int

const (
	// DismissHorizontal allows a swipe in either horizontal direction.
	DismissHorizontal DismissDir = iota
	// DismissLeft allows only a leftward swipe (content exits left).
	DismissLeft
	// DismissRight allows only a rightward swipe (content exits right).
	DismissRight
)

// Dismissible lets its child be swiped horizontally off screen — the row
// follows the finger, and a swipe past the threshold (or a fast flick)
// animates it out and calls OnDismissed; a short swipe springs back. Put one
// per list item (behind a stable WithKey) and remove the item from your model
// in OnDismissed.
type Dismissible struct {
	Child Widget
	// Background shows behind the child as it slides aside (e.g. a red
	// delete panel). Optional.
	Background  Widget
	Direction   DismissDir
	OnDismissed func()
	// Threshold is the fraction of width the swipe must pass to dismiss on
	// release (0 → 0.4).
	Threshold float32
}

func (d Dismissible) CreateState() State { return &dismissState{} }

type dismissState struct {
	StateBase[Dismissible]
	ctx      Ctx
	dx       float32 // current horizontal translation (px)
	width    float32 // last laid-out child width
	velocity float32 // px/s along x
	lastDrag time.Time
	anim     *anim.Controller
	from, to float32
	gone     bool // dismiss animation is running/finished
	fired    bool // OnDismissed already called
}

func (s *dismissState) Init(ctx Ctx) {
	s.ctx = ctx
	s.anim = &anim.Controller{Curve: anim.EaseOut, Duration: 220 * time.Millisecond, OnChange: func() {
		s.SetState(func() { s.dx = geom.LerpFloat(s.from, s.to, s.anim.Value()) })
		if !s.anim.Running() && s.gone && !s.fired {
			s.fired = true
			if cb := s.W().OnDismissed; cb != nil {
				cb()
			}
		}
	}}
	ctx.AddTicker(s.anim)
}

func (s *dismissState) Dispose() { s.ctx.RemoveTicker(s.anim) }

func (s *dismissState) allow(dx float32) float32 {
	switch s.W().Direction {
	case DismissLeft:
		return min(dx, 0)
	case DismissRight:
		return max(dx, 0)
	}
	return dx
}

func (s *dismissState) animateTo(to float32, dismiss bool) {
	s.gone = dismiss
	s.from, s.to = s.dx, to
	s.anim.Jump(0)
	s.anim.Forward()
	s.ctx.Invalidate()
}

func (s *dismissState) release() {
	if s.gone {
		return
	}
	thr := s.W().Threshold
	if thr <= 0 {
		thr = 0.4
	}
	trigger := thr * s.width
	past := s.width > 0 && (s.dx > trigger || s.dx < -trigger)
	flick := s.velocity > 900 || s.velocity < -900
	if (past || flick) && s.width > 0 {
		dir := float32(1)
		if s.dx < 0 || (s.dx == 0 && s.velocity < 0) {
			dir = -1
		}
		s.animateTo(dir*s.width, true)
	} else {
		s.animateTo(0, false)
	}
}

func (s *dismissState) Build(ctx Ctx) Widget {
	w := s.W()
	slide := slideBox{dx: s.dx, out: &s.width, child: Interactive{
		Handler: Handler{
			DragAxis: DragHorizontal, // let a vertical scroll claim vertical drags
			OnPress: func(geom.Pt) {
				s.anim.Jump(1) // grab interrupts any spring/dismiss
				s.velocity, s.lastDrag = 0, time.Now()
			},
			OnDrag: func(_, d geom.Pt) {
				if s.gone {
					return
				}
				s.SetState(func() { s.dx = s.allow(s.dx + d.X) })
				now := time.Now()
				dt := now.Sub(s.lastDrag).Seconds()
				s.lastDrag = now
				if dt > 0.001 && dt < 0.1 {
					s.velocity = s.velocity*0.7 + (d.X/float32(dt))*0.3
				}
			},
			OnRelease: func() { s.release() },
		},
		Child: w.Child,
	}}
	if w.Background == nil {
		return slide
	}
	// Reveal the background (stretched to the row) only while the row is
	// displaced — otherwise the row's own rounded corners would let the panel
	// peek through at rest.
	return dismissStack{bg: w.Background, fg: slide, reveal: s.dx != 0}
}

// dismissStack paints its background stretched to exactly the foreground's
// size, then the foreground on top — so a swipe reveals a full-height panel
// (unlike Stack, which sizes each layer to its own content).
type dismissStack struct {
	bg, fg Widget
	reveal bool
}

func (d dismissStack) createBox(Ctx) layout.Box      { return &dismissBox{} }
func (d dismissStack) updateBox(_ Ctx, b layout.Box) { b.(*dismissBox).reveal = d.reveal }
func (d dismissStack) childWidgets() []Widget        { return []Widget{d.bg, d.fg} }
func (d dismissStack) attach(b layout.Box, kids []layout.Box) {
	db := b.(*dismissBox)
	db.bg, db.fg = kids[0], kids[1]
}

type dismissBox struct {
	layout.Base
	bg, fg layout.Box
	reveal bool
	size   geom.Size
}

func (b *dismissBox) Layout(cs layout.Constraints) geom.Size {
	if b.fg != nil {
		b.size = b.fg.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	if b.bg != nil {
		b.bg.Layout(layout.Tight(b.size))
	}
	return b.size
}

func (b *dismissBox) Size() geom.Size { return b.size }

func (b *dismissBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.bg != nil && b.reveal {
		b.bg.Paint(c, at)
	}
	if b.fg != nil {
		b.fg.Paint(c, at)
	}
}

func (b *dismissBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.fg != nil {
		b.fg.AddHits(p, hits)
	}
}

func (b *dismissBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.bg != nil {
		visit(b.bg, geom.Pt{})
	}
	if b.fg != nil {
		visit(b.fg, geom.Pt{})
	}
}

// slideBox translates its child horizontally and reports the laid-out width
// back through out (the previous frame's width, valid for threshold math).
type slideBox struct {
	dx    float32
	out   *float32
	child Widget
}

func (b slideBox) createBox(Ctx) layout.Box { return &layout.Translated{} }
func (b slideBox) updateBox(_ Ctx, box layout.Box) {
	t := box.(*layout.Translated)
	t.Dx = b.dx
	if b.out != nil {
		*b.out = t.Size().W
	}
}
func (b slideBox) childWidgets() []Widget { return []Widget{b.child} }
func (b slideBox) attach(box layout.Box, kids []layout.Box) {
	box.(*layout.Translated).Child = first(kids)
}
