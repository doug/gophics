package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// Drag and drop. A Draggable picks its payload up and carries a floating
// preview under the pointer; DropTargets underneath report when the payload is
// over them and receive it on release.
//
// The two halves talk through a DragSession provided at the app root. That
// indirection is what lets a target react to a drag it never sees the gesture
// for: gophics routes a pointer gesture to one handler for its whole duration,
// so a target cannot learn about a drag by waiting to be hovered. The session
// is the shared blackboard the Draggable writes and the targets read.
//
// Payloads are `any`, matched by the target's Accept function, so a tree can
// carry several kinds of drag without a type registry.

// DragSession is the app-wide drag state. Install one with DragHost (the app
// runner does this at the root, next to OverlayHost); widgets reach it with
// widget.Of[*DragSession].
type DragSession struct {
	// payload is the value being carried, nil when no drag is in flight.
	payload any
	// pos is the pointer position in root coordinates.
	pos geom.Pt
	// over is the target currently under the pointer, for enter/leave.
	over *dropReg
	// targets are the registered drop targets, in registration order.
	targets []*dropReg
	// repaint asks the host to rebuild so previews and highlights update.
	repaint func()
}

// dropReg is one registered target's live state.
type dropReg struct {
	accept func(payload any) bool
	drop   func(payload any, at geom.Pt)
	// rect is the target's bounds in root coordinates, refreshed each frame
	// by the target's own render object.
	rect geom.Rect
	// hovered is true while a compatible payload is over this target.
	hovered bool
}

// Dragging reports whether a drag is in flight.
func (s *DragSession) Dragging() bool { return s.payload != nil }

// Payload returns the value being carried, or nil.
func (s *DragSession) Payload() any { return s.payload }

// Pos returns the pointer position in root coordinates.
func (s *DragSession) Pos() geom.Pt { return s.pos }

func (s *DragSession) begin(payload any, at geom.Pt) {
	s.payload, s.pos = payload, at
	s.update(at)
}

// update moves the drag and recomputes which target is under the pointer.
func (s *DragSession) update(at geom.Pt) {
	s.pos = at
	var hit *dropReg
	// Later registrations sit on top, matching paint order, so the last
	// matching target under the pointer wins.
	for _, t := range s.targets {
		if t.accept != nil && !t.accept(s.payload) {
			continue
		}
		if t.rect.Contains(at) {
			hit = t
		}
	}
	if hit == s.over {
		return
	}
	if s.over != nil {
		s.over.hovered = false
	}
	s.over = hit
	if hit != nil {
		hit.hovered = true
	}
	s.notify()
}

// end drops the payload on the target under the pointer, if any.
func (s *DragSession) end(at geom.Pt) {
	s.update(at)
	target, payload := s.over, s.payload
	if s.over != nil {
		s.over.hovered = false
	}
	s.over, s.payload = nil, nil
	if target != nil && target.drop != nil {
		target.drop(payload, at)
	}
	s.notify()
}

// cancel abandons the drag without delivering it.
func (s *DragSession) cancel() {
	if s.payload == nil {
		return
	}
	if s.over != nil {
		s.over.hovered = false
	}
	s.over, s.payload = nil, nil
	s.notify()
}

func (s *DragSession) notify() {
	if s.repaint != nil {
		s.repaint()
	}
}

func (s *DragSession) register(r *dropReg) {
	for _, t := range s.targets {
		if t == r {
			return
		}
	}
	s.targets = append(s.targets, r)
}

func (s *DragSession) unregister(r *dropReg) {
	for i, t := range s.targets {
		if t == r {
			s.targets = append(s.targets[:i], s.targets[i+1:]...)
			break
		}
	}
	if s.over == r {
		s.over = nil
	}
}

// DragHost provides a DragSession to its subtree and paints the in-flight
// preview above everything else.
type DragHost struct{ Child Widget }

func (DragHost) CreateState() State { return &dragHostState{} }

type dragHostState struct {
	StateBase[DragHost]
	sess DragSession
}

func (s *dragHostState) Init(Ctx) {
	s.sess.repaint = func() { s.SetState(nil) }
}

func (s *dragHostState) Build(ctx Ctx) Widget {
	return Provide[*DragSession]{Value: &s.sess, Child: s.W().Child}
}

// Draggable makes its child the handle for dragging Payload.
//
// The drag starts after the pointer has moved past a short threshold, so a tap
// on a draggable row is still a tap. On touch, require a long press first
// (LongPressToStart) or a list will pick up items whenever it is scrolled.
type Draggable struct {
	// Payload is the value delivered to the DropTarget. Required.
	Payload any
	// Preview is what follows the pointer. Nil uses Child, which is the right
	// default: what you picked up is what you are carrying.
	Preview Widget
	// PreviewOffset shifts the preview relative to the pointer. Zero centers
	// it on the pointer.
	PreviewOffset geom.Pt
	// LongPressToStart requires a long press before the drag begins. Set it
	// for anything inside a scrollable, where a plain drag means scroll.
	LongPressToStart bool
	// OnDragStart and OnDragEnd bracket the gesture; dropped reports whether
	// a target took the payload.
	OnDragStart func()
	OnDragEnd   func(dropped bool)
	Child       Widget
}

func (d Draggable) CreateState() State { return &draggableState{} }

type draggableState struct {
	StateBase[Draggable]
	ctx     Ctx
	sess    *DragSession
	armed   bool // long-press satisfied (or not required)
	active  bool
	tok     OverlayToken
	origin  geom.Pt // pointer position in root coords at press
	pressAt geom.Pt // press position in local coords
}

func (s *draggableState) Init(ctx Ctx) {
	s.ctx = ctx
	s.sess, _ = Of[*DragSession](ctx)
	s.armed = !s.W().LongPressToStart
}

func (s *draggableState) Dispose() {
	// A widget removed mid-drag must not leave the session holding a payload
	// nobody can drop.
	if s.active && s.sess != nil {
		s.sess.cancel()
		s.tok.Dismiss()
	}
}

// dragSlop is how far the pointer must move before a press becomes a drag.
const dragSlop = 6

func (s *draggableState) start() {
	if s.active || s.sess == nil || s.W().Payload == nil {
		return
	}
	s.active = true
	s.sess.begin(s.W().Payload, s.origin)
	// The moment the item leaves the surface is the one worth feeling: on
	// touch it is the only confirmation that the long press took, since the
	// finger is covering what just changed.
	s.haptic(shell.HapticMedium)
	if f := s.W().OnDragStart; f != nil {
		f()
	}
	s.showPreview()
}

// haptic plays k if the platform has haptics (desktop does not).
func (s *draggableState) haptic(k shell.HapticKind) {
	if h := s.ctx.Haptic(); h != nil {
		h.Play(k)
	}
}

// showPreview puts the carried widget in the overlay, positioned by padding
// from the top-left — the same mechanism menus and tooltips use.
func (s *draggableState) showPreview() {
	ov, ok := Of[Overlay](s.ctx)
	if !ok {
		return
	}
	preview := s.W().Preview
	if preview == nil {
		preview = s.W().Child
	}
	s.tok = ov.Show(dragPreview{sess: s.sess, offset: s.W().PreviewOffset, Child: preview})
}

func (s *draggableState) finish(dropped bool) {
	if !s.active {
		return
	}
	s.active = false
	s.tok.Dismiss()
	// Landing and failing to land should not feel the same.
	if dropped {
		s.haptic(shell.HapticSuccess)
	} else {
		s.haptic(shell.HapticLight)
	}
	if f := s.W().OnDragEnd; f != nil {
		f(dropped)
	}
}

func (s *draggableState) Build(ctx Ctx) Widget {
	w := s.W()
	return Interactive{
		Handler: Handler{
			DragAxis: DragAny,
			// Until the long press arms it, this drag is not ours: stand down
			// so the page can scroll under a finger that starts on a chip.
			DragClaims: func(bool) bool { return s.armed },
			OnPress: func(local geom.Pt) {
				s.pressAt = local
				s.origin = ctx.Input().Pointer()
				s.armed = !w.LongPressToStart
			},
			OnLongPress: func() {
				if w.LongPressToStart {
					s.armed = true
					s.origin = ctx.Input().Pointer()
					s.start()
				}
			},
			// A drag that has not passed the slop yet is still a candidate
			// tap, so the session is not touched until it commits.
			OnDrag: func(local, _ geom.Pt) {
				if !s.armed {
					return
				}
				at := ctx.Input().Pointer()
				if !s.active {
					if dist2(at, s.origin) < dragSlop*dragSlop {
						return
					}
					s.start()
				}
				s.sess.update(at)
			},
			OnRelease: func() {
				if !s.active {
					return
				}
				at := ctx.Input().Pointer()
				dropped := s.sess.overTarget()
				s.sess.end(at)
				s.finish(dropped)
			},
			OnPressEnd: func() {
				// Covers a cancelled gesture (focus loss, a steal) that never
				// reaches OnRelease.
				if s.active {
					s.sess.cancel()
					s.finish(false)
				}
			},
		},
		Child: w.Child,
	}
}

// overTarget reports whether a compatible target is currently under the
// pointer — asked before end() clears the state.
func (s *DragSession) overTarget() bool { return s.over != nil }

func dist2(a, b geom.Pt) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx + dy*dy
}

// dragPreview positions the carried widget under the pointer each frame.
type dragPreview struct {
	sess   *DragSession
	offset geom.Pt
	Child  Widget
}

func (p dragPreview) Build(Ctx) Widget {
	if p.sess == nil || !p.sess.Dragging() {
		return Sized{}
	}
	at := p.sess.Pos().Add(p.offset)
	// Padding cannot take a negative inset, so the pointer position places the
	// box's top-left and centred shifts it back by half its own size — which is
	// only knowable at layout time. Without this the ghost hangs below and to
	// the right of the finger rather than under it, which is what
	// PreviewOffset has always documented ("zero centers it on the pointer").
	return Padding{
		Insets: geom.Insets{Left: max0(at.X), Top: max0(at.Y)},
		Child:  Align{X: 0, Y: 0, Child: centered{child: Opacity{Alpha: 0.85, Child: p.Child}}},
	}
}

// centered shifts a child back by half its own size, so it sits centred on
// whatever point positioned it.
type centered struct{ child Widget }

func (c centered) createBox(Ctx) layout.Box { return &layout.Translated{} }
func (c centered) updateBox(_ Ctx, b layout.Box) {
	t := b.(*layout.Translated)
	t.FracX, t.FracY = -0.5, -0.5
}
func (c centered) childWidgets() []Widget { return []Widget{c.child} }
func (c centered) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Translated).Child = first(kids)
}

func max0(v float32) float32 {
	if v < 0 {
		return 0
	}
	return v
}

// DropTarget receives a dragged payload released over it.
type DropTarget struct {
	// Accept decides whether this target wants a payload. Nil accepts any
	// non-nil payload, which is only right in a tree with one drag type.
	Accept func(payload any) bool
	// OnDrop receives the payload, with the drop point in root coordinates.
	OnDrop func(payload any, at geom.Pt)
	// Builder renders the target. hovering reports that a compatible payload
	// is over it, which is how a target shows it will accept the drop. Nil
	// falls back to Child unchanged.
	Builder func(hovering bool) Widget
	Child   Widget
}

func (d DropTarget) CreateState() State { return &dropTargetState{} }

type dropTargetState struct {
	StateBase[DropTarget]
	ctx  Ctx
	sess *DragSession
	reg  dropReg
	last bool // last hovered value, to rebuild only on change
}

func (s *dropTargetState) Init(ctx Ctx) {
	s.ctx = ctx
	s.sess, _ = Of[*DragSession](ctx)
	s.reg.accept = func(p any) bool {
		if p == nil {
			return false
		}
		if a := s.W().Accept; a != nil {
			return a(p)
		}
		return true
	}
	s.reg.drop = func(p any, at geom.Pt) {
		if f := s.W().OnDrop; f != nil {
			f(p, at)
		}
	}
	if s.sess != nil {
		s.sess.register(&s.reg)
	}
}

func (s *dropTargetState) Dispose() {
	if s.sess != nil {
		s.sess.unregister(&s.reg)
	}
}

func (s *dropTargetState) Build(ctx Ctx) Widget {
	w := s.W()
	child := w.Child
	if w.Builder != nil {
		child = w.Builder(s.reg.hovered)
	}
	// dropZone reports its root-space rect back into the registration every
	// frame, which is what lets the session hit-test targets it is not
	// receiving gestures for.
	return dropZone{reg: &s.reg, onHoverChange: func() { s.SetState(nil) }, Child: child}
}

// dropZone is the render object that keeps a target's bounds current.
type dropZone struct {
	reg           *dropReg
	onHoverChange func()
	Child         Widget
}

func (z dropZone) createBox(Ctx) layout.Box { return &dropZoneBox{} }
func (z dropZone) updateBox(_ Ctx, b layout.Box) {
	zb := b.(*dropZoneBox)
	zb.reg = z.reg
}
func (z dropZone) childWidgets() []Widget { return []Widget{z.Child} }
func (z dropZone) soleChild() Widget      { return z.Child }
func (z dropZone) attach(b layout.Box, kids []layout.Box) {
	b.(*dropZoneBox).Child = first(kids)
}

type dropZoneBox struct {
	reg   *dropReg
	Child layout.Box
	size  geom.Size
}

func (b *dropZoneBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *dropZoneBox) Size() geom.Size { return b.size }

// Paint is where the root-space rect is captured: it is the one pass that
// knows a box's absolute position, and it runs every frame the target is
// visible. A target scrolled off screen stops painting and stops matching,
// which is the behavior you want anyway.
func (b *dropZoneBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reg != nil {
		b.reg.rect = geom.Rect{Min: at, Max: at.Add(b.size.Pt())}
	}
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *dropZoneBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (b *dropZoneBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X < 0 || p.Y < 0 || p.X >= b.size.W || p.Y >= b.size.H {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
}
