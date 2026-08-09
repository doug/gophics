package widget

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
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

	edgeDx float32 // accumulated left-edge back-swipe distance
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

// settle synchronously completes an in-flight transition, applying the stack
// mutation its completion handler would have applied (a pop's truncation).
// push/pop call it before starting a new transition so a navigation landing
// mid-animation proceeds from a consistent stack — without it, a push during a
// pop would replace s.trans and the pop's truncation would silently never run
// (the popped page stayed retained). Call only from inside SetState.
func (s *navState) settle() {
	if s.trans == nil {
		return
	}
	if s.trans.popping {
		s.stack = s.stack[:len(s.stack)-1]
	}
	s.trans = nil
	s.underReg, s.overReg = nil, nil
	s.animating = false
}

func (s *navState) push(w Widget) {
	s.SetState(func() {
		s.settle()
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
	if len(s.stack) == 0 && s.trans == nil {
		return
	}
	s.SetState(func() {
		s.settle()
		if len(s.stack) == 0 {
			return // settling an in-flight pop emptied the stack
		}
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
		// Every page is wrapped in the same keyed pageW shape — one wrapper
		// type whatever the page's role (offstage, resting, transitioning),
		// with the role expressed as fields. A page therefore keeps its
		// element — and its state (scroll, loaded data) — as it moves between
		// roles; a wrapper type that changed with the role would remount it.
		var provReg *heroRegistry // hero registry provided to the page's subtree
		var slideReg *heroRegistry
		var frac float32
		if s.trans != nil {
			switch i {
			case len(pages) - 1:
				provReg, slideReg = s.overReg, s.overReg
				t := s.slide.Value()
				frac = 1 - t // push: sliding in from the right
				if s.trans.popping {
					frac = t // pop: sliding out to the right
				}
			case visibleFrom:
				provReg = s.underReg
			}
		}
		children = append(children, WithKey{Key: i, Child: pageW{
			offstage: i < visibleFrom,
			fracX:    frac,
			reg:      slideReg,
			child:    Provide[*heroRegistry]{Value: provReg, Child: p},
		}})
	}
	// Hero flight overlay: a LayoutBuilder gives the surface width needed to
	// undo page slides when recovering at-rest hero rects.
	if s.trans != nil {
		children = append(children, LayoutBuilder{Build: func(cs layout.Constraints) Widget {
			return stackW{Children: s.buildFlights(cs.Max.W)}
		}})
	}
	// Back-swipe: a left-edge, horizontal-only drag strip that pops when the
	// swipe passes the threshold (iOS-style). Topmost so it wins the edge over
	// a card's swipe-to-dismiss; DragHorizontal so vertical scrolls pass
	// through. Only present when there's something to pop.
	if len(s.stack) > 0 && s.trans == nil {
		children = append(children, Interactive{
			Handler: Handler{
				DragAxis: DragHorizontal,
				OnPress:  func(geom.Pt) { s.edgeDx = 0 },
				OnDrag:   func(_, d geom.Pt) { s.edgeDx += d.X },
				OnRelease: func() {
					if s.edgeDx > 64 {
						s.pop()
					}
					s.edgeDx = 0
				},
			},
			Child: Sized{W: 22, H: 1e6}, // a full-height left-edge strip (clamped)
		})
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

func (w stackW) createBox(Ctx) layout.Box      { return &layout.Stack{} }
func (w stackW) updateBox(_ Ctx, b layout.Box) {}
func (w stackW) childWidgets() []Widget        { return w.Children }
func (w stackW) attach(b layout.Box, kids []layout.Box) {
	st := b.(*layout.Stack)
	st.Children = append(st.Children[:0], kids...)
}

// pageW is the uniform wrapper around every page in the stack. All roles run
// through the one type: offstage pages stay mounted and laid out but neither
// paint nor hit-test; the transitioning top page is translated by fracX and
// records the applied fraction into its hero registry during paint (so
// restRect can recover its heroes' at-rest rects without frame lag).
type pageW struct {
	offstage bool
	fracX    float32
	reg      *heroRegistry // non-nil only for the transitioning top page
	child    Widget
}

func (w pageW) createBox(Ctx) layout.Box { return &pageBox{} }
func (w pageW) updateBox(_ Ctx, b layout.Box) {
	pb := b.(*pageBox)
	pb.offstage, pb.fracX, pb.reg = w.offstage, w.fracX, w.reg
}
func (w pageW) childWidgets() []Widget { return []Widget{w.child} }
func (w pageW) attach(b layout.Box, kids []layout.Box) {
	b.(*pageBox).child = first(kids)
}

type pageBox struct {
	layout.Base
	offstage bool
	fracX    float32
	reg      *heroRegistry
	child    layout.Box
}

func (b *pageBox) offset() geom.Pt { return geom.Pt{X: b.fracX * b.Size().W} }

func (b *pageBox) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.child != nil {
		return b.Done(cs, b.child.Layout(cs))
	}
	return b.Done(cs, cs.Constrain(geom.Size{}))
}

func (b *pageBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.offstage {
		return
	}
	if b.reg != nil {
		b.reg.frac = b.fracX // matches the rects captured below this paint
	}
	if b.child != nil {
		b.child.Paint(c, at.Add(b.offset()))
	}
}

func (b *pageBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.offstage {
		return
	}
	if b.child != nil {
		b.child.AddHits(p.Sub(b.offset()), hits)
	}
}

func (b *pageBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.offstage || b.child == nil {
		return
	}
	visit(b.child, b.offset())
}

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
