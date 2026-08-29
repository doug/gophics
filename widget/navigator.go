package widget

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Navigator manages a page stack with slide transitions. Pages reach it
// through the provided Nav handle:
//
//	nav := ctx.MustOf[widget.Nav]()
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
	// Ignore a push while another push is still animating: two quick taps on a
	// list row would otherwise open the same route twice, and the user has to
	// press back twice to escape. The transition is exactly the window in which a
	// tap is easiest to repeat by accident, because nothing has visibly finished
	// yet, and platform navigators drop the second tap for the same reason.
	//
	// Only push-during-push. Interleaving directions is deliberate and tested: a
	// push during a pop (or the reverse) settles the in-flight transition and
	// starts the new one, which is how a "go back and immediately elsewhere"
	// gesture is meant to behave.
	if s.animating && s.trans != nil && !s.trans.popping {
		return
	}
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
// parallaxFrac is how far the outgoing page travels, as a fraction of the
// surface width, while the incoming page crosses the whole of it. A third is
// the proportion iOS and Material both settle near; far enough that the two
// pages are visibly separate, short enough that the outgoing one never leaves.
const parallaxFrac = 0.30

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
			t := s.slide.Value()
			over, under := pageFracs(t, s.trans.popping)
			switch i {
			case len(pages) - 1:
				provReg, slideReg = s.overReg, s.overReg
				frac = over // sliding in from the right (out to it, on a pop)
			case visibleFrom:
				// The page being left behind eases aside rather than sitting
				// still. Without this the incoming page slides over a fixed
				// background, which reads as one page popping on top of
				// another instead of as a stack moving: the eye follows two
				// things travelling together at different rates, and a single
				// moving layer over a static one is the thing it does not do.
				//
				// It travels a fraction of the width, so the pages separate as
				// they move; matching speeds would look like one wide image
				// sliding past.
				provReg, slideReg = s.underReg, s.underReg
				frac = under
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
			Gestures: Gestures{
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

// pageFracs is the main-axis slide of the incoming and outgoing pages at t,
// as a fraction of the surface width. Build positions the pages by it and
// buildFlights places the hero overlay by it, so the two cannot drift.
func pageFracs(t float32, popping bool) (over, under float32) {
	if popping {
		return t, -parallaxFrac * (1 - t)
	}
	return 1 - t, -parallaxFrac * t
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
	over, under := pageFracs(t, s.trans.popping)
	fromFrac, toFrac := under, over // push: under → over
	if s.trans.popping {
		fromFrac, toFrac = over, under
	}

	var flights []Widget
	for tag, toRC := range to.rects {
		fromRC, ok := from.rects[tag]
		if !ok {
			continue
		}
		// Normalise to at-rest (the painted rect carries whatever slide the
		// page had when it was captured, and a hero stops painting once it is
		// flying, so its raw rect goes stale), then re-apply the slide each
		// page has *this* frame. Interpolating in at-rest space alone put the
		// flight where the destination page was going to be rather than where
		// it currently was, so the element sat offset from its own page for
		// the whole transition and only met it on the last frame.
		src := restRect(fromRC, from.frac, width).Translate(geom.Pt{X: fromFrac * width})
		dst := restRect(toRC, to.frac, width).Translate(geom.Pt{X: toFrac * width})
		// Into the stack's coordinates. The rects came from paint and are in
		// window coordinates; the overlay below is aligned to the stack's own
		// top-left. Under a header — or inside a notch's safe-area inset —
		// those differ, and the flight arrives offset by the difference
		// instead of landing on the element it is flying to.
		if o := to.origin; o != (geom.Pt{}) {
			src = src.Translate(geom.Pt{X: -o.X, Y: -o.Y})
			dst = dst.Translate(geom.Pt{X: -o.X, Y: -o.Y})
		}
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

func (w stackW) createBox(Ctx) layout.Box      { return &layoutbox.Stack{} }
func (w stackW) updateBox(_ Ctx, b layout.Box) {}
func (w stackW) childWidgets() []Widget        { return w.Children }
func (w stackW) attach(b layout.Box, kids []layout.Box) {
	st := b.(*layoutbox.Stack)
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
func (w pageW) soleChild() Widget      { return w.child }
func (w pageW) attach(b layout.Box, kids []layout.Box) {
	b.(*pageBox).child = first(kids)
}

type pageBox struct {
	layoutbox.Base
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
		// The stack's own origin, which is simply where this page's box was
		// placed: the slide is applied to the child below, not to the box. Do
		// not subtract it here — restRect already removes the slide from the
		// captured rect, so subtracting it again moves the flight by the slide
		// a second time and the hero drifts during every transition even when
		// the navigator starts at the window origin.
		b.reg.origin = at
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

func (w translatedW) createBox(Ctx) layout.Box { return &layoutbox.Translated{} }
func (w translatedW) updateBox(_ Ctx, b layout.Box) {
	b.(*layoutbox.Translated).FracX = w.FracX
}
func (w translatedW) childWidgets() []Widget { return []Widget{w.Child} }
func (w translatedW) soleChild() Widget      { return w.Child }
func (w translatedW) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Translated).Child = first(kids)
}
