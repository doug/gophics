package widget

import (
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// Navigator manages a page stack with slide transitions. Pages reach it
// through the provided Nav handle:
//
//	nav := widget.MustOf[widget.Nav](ctx)
//	nav.Push(DetailPage{...})
//	nav.Pop()
//
// Home is the root page (never popped). Pushed pages slide in from the
// right over the previous page; Pop slides them out.
type Navigator struct {
	Home Widget
}

// Nav is the navigation handle available below a Navigator via Of/MustOf.
type Nav struct{ s *navState }

// Push animates w onto the stack.
func (n Nav) Push(w Widget) { n.s.push(w) }

// Pop animates the top page off; on the root page it does nothing.
func (n Nav) Pop() { n.s.pop() }

// Depth is the current stack depth (1 = home only).
func (n Nav) Depth() int { return len(n.s.stack) + 1 }

func (nv Navigator) CreateState() State { return &navState{} }

type transition struct {
	under, over Widget
	popping     bool
}

type navState struct {
	StateBase[Navigator]
	ctx       Ctx
	stack     []Widget // pushed pages; Home is implicit below
	slide     *anim.Controller
	trans     *transition
	animating bool

	// Hero-flight coordination during a transition: each transitioning page
	// gets its own registry (via Provide) so its heroes report rects and get
	// suppressed while the flight overlay animates the shared element.
	underReg, overReg *heroRegistry
}

func (s *navState) Init(ctx Ctx) {
	s.ctx = ctx
	s.slide = &anim.Controller{
		Duration: 220 * time.Millisecond,
		Curve:    anim.EaseOut,
		OnChange: func() {
			// Completion only ends a *running* transition; Jump(0) during
			// setup also fires OnChange and must not finish it early.
			if s.animating && !s.slide.Running() && s.trans != nil {
				s.animating = false
				s.SetState(func() {
					if s.trans.popping {
						s.stack = s.stack[:len(s.stack)-1]
					}
					s.trans = nil
					s.underReg, s.overReg = nil, nil
				})
				return
			}
			s.SetState(nil)
		},
	}
	ctx.AddTicker(s.slide)
}

func (s *navState) Dispose() { s.ctx.RemoveTicker(s.slide) }

func (s *navState) top() Widget {
	if len(s.stack) > 0 {
		return s.stack[len(s.stack)-1]
	}
	return s.W().Home
}

func (s *navState) push(w Widget) {
	s.SetState(func() {
		under := s.top()
		s.stack = append(s.stack, w)
		s.trans = &transition{under: under, over: w}
		s.underReg, s.overReg = newHeroRegistry(), newHeroRegistry()
		s.slide.Jump(0)
		s.slide.Forward()
		s.animating = true
	})
	s.ctx.Invalidate()
}

func (s *navState) pop() {
	if len(s.stack) == 0 || s.trans != nil {
		return
	}
	s.SetState(func() {
		over := s.top()
		under := s.W().Home
		if len(s.stack) > 1 {
			under = s.stack[len(s.stack)-2]
		}
		s.trans = &transition{under: under, over: over, popping: true}
		s.underReg, s.overReg = newHeroRegistry(), newHeroRegistry()
		s.slide.Jump(0)
		s.slide.Forward()
		s.animating = true
	})
	s.ctx.Invalidate()
}

// Build keeps every page mounted (state — scroll positions, loaded data —
// survives under the stack); pages below the visible ones are offstage:
// laid out but neither painted nor hit-testable.
func (s *navState) Build(Ctx) Widget {
	pages := append([]Widget{s.W().Home}, s.stack...)
	children := make([]Widget, 0, len(pages)+1)

	visibleFrom := len(pages) - 1 // index of the lowest *visible* page
	if s.trans != nil {
		visibleFrom-- // the page under the transition shows too
		if visibleFrom < 0 {
			visibleFrom = 0
		}
	}
	for i, p := range pages {
		// Every page is wrapped in the same Provide[*heroRegistry] shape
		// (nil outside its transition role) so a page keeps its element — and
		// its state (scroll, loaded data) — as it moves between roles; a
		// wrapper that appeared only during a transition would remount it.
		var reg *heroRegistry
		if s.trans != nil {
			switch i {
			case len(pages) - 1:
				reg = s.overReg
			case visibleFrom:
				reg = s.underReg
			}
		}
		page := Provide[*heroRegistry]{Value: reg, Child: WithKey{Key: i, Child: p}}
		switch {
		case i < visibleFrom:
			children = append(children, offstageW{Child: page})
		case s.trans != nil && i == len(pages)-1:
			t := s.slide.Value()
			frac := 1 - t // push: sliding in from the right
			if s.trans.popping {
				frac = t // pop: sliding out to the right
			}
			children = append(children, heroPageW{fracX: frac, reg: s.overReg, child: page})
		default:
			children = append(children, page)
		}
	}
	// Hero flight overlay: a LayoutBuilder gives the surface width needed to
	// undo page slides when recovering at-rest hero rects.
	if s.trans != nil {
		children = append(children, LayoutBuilder{Build: func(cs layout.Constraints) Widget {
			return stackW{Children: s.buildFlights(cs.Max.W)}
		}})
	}
	return Provide[Nav]{Value: Nav{s: s}, Child: stackW{Children: children}}
}

// buildFlights returns the shared-element flight widgets for the current
// transition: for each tag present on both the outgoing and incoming pages,
// an overlay copy interpolating from one rect to the other, and it flags both
// real heroes to suppress themselves. Rects come from the previous frame's
// paint, so the flight appears one frame into the transition.
func (s *navState) buildFlights(width float32) []Widget {
	if s.underReg == nil || s.overReg == nil {
		return nil
	}
	// The element travels from its rect on the outgoing page to the incoming.
	from, to := s.underReg, s.overReg // push: under → over
	if s.trans.popping {
		from, to = s.overReg, s.underReg // pop: over → under
	}
	t := s.slide.Value()

	var flights []Widget
	for tag, toRC := range to.rects {
		fromRC, ok := from.rects[tag]
		if !ok {
			continue
		}
		src := restRect(fromRC, from.frac, width)
		dst := restRect(toRC, to.frac, width)
		from.flying[tag], to.flying[tag] = true, true
		cur := lerpRect(src, dst, t)
		// The child is rebuilt at the origin (its natural size ≈ dst size);
		// MapRect scales/moves it onto the interpolated rect.
		child := to.child[tag]
		flights = append(flights, Align{X: 0, Y: 0, Child: Transform{
			T:     paint.MapRect(geom.RectFromSize(dst.Size()), cur),
			Child: child,
		}})
	}
	return flights
}

// stackW and translatedW are internal render widgets over the layout boxes.
type stackW struct{ Children []Widget }

func (w stackW) createBox(Ctx) layout.Box          { return &layout.Stack{} }
func (w stackW) updateBox(_ Ctx, b layout.Box)     {}
func (w stackW) childWidgets() []Widget            { return w.Children }
func (w stackW) attach(b layout.Box, kids []layout.Box) {
	st := b.(*layout.Stack)
	st.Children = append(st.Children[:0], kids...)
}

// offstageW keeps its child mounted and laid out without painting or
// hit-testing it.
type offstageW struct{ Child Widget }

func (w offstageW) createBox(Ctx) layout.Box      { return &offstageBox{} }
func (w offstageW) updateBox(Ctx, layout.Box)     {}
func (w offstageW) childWidgets() []Widget        { return []Widget{w.Child} }
func (w offstageW) attach(b layout.Box, kids []layout.Box) {
	b.(*offstageBox).Child = first(kids)
}

type offstageBox struct {
	layout.Base
	Child layout.Box
}

func (b *offstageBox) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child != nil {
		return b.Done(cs, b.Child.Layout(cs))
	}
	return b.Done(cs, cs.Constrain(geom.Size{}))
}

func (b *offstageBox) Paint(paint.Canvas, geom.Pt)      {}
func (b *offstageBox) AddHits(geom.Pt, *[]layout.Hit)   {}

type translatedW struct {
	FracX float32
	Child Widget
}

func (w translatedW) createBox(Ctx) layout.Box { return &layout.Translated{} }
func (w translatedW) updateBox(_ Ctx, b layout.Box) {
	b.(*layout.Translated).FracX = w.FracX
}
func (w translatedW) childWidgets() []Widget { return []Widget{w.Child} }
func (w translatedW) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Translated).Child = first(kids)
}
