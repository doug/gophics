package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// LayoutBuilder builds its child from the constraints it's given — the
// responsive primitive. Branch on the available width to lay out a phone
// vs. a desktop pane from one widget:
//
//	widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
//	    if cs.Max.W > 600 { return wideLayout }
//	    return narrowLayout
//	}}
//
// It observes the constraints during layout and rebuilds when they change,
// so it settles one frame after a resize (imperceptible in practice; a
// resize already triggers a frame). On first mount the child appears on the
// second frame.
type LayoutBuilder struct {
	Build func(cs layout.Constraints) Widget
}

func (LayoutBuilder) CreateState() State { return &lbState{} }

type lbState struct {
	StateBase[LayoutBuilder]
	cs     layout.Constraints
	haveCS bool
}

func (s *lbState) Build(Ctx) Widget {
	var child Widget
	if s.haveCS && s.W().Build != nil {
		child = s.W().Build(s.cs)
	}
	return constraintObserver{
		onCS: func(cs layout.Constraints) {
			if !s.haveCS || cs != s.cs {
				s.SetState(func() { s.cs, s.haveCS = cs, true })
			}
		},
		Child: child,
	}
}

// constraintObserver is a render widget that reports the constraints its box
// receives during layout, then lays out its child with them.
type constraintObserver struct {
	onCS  func(layout.Constraints)
	Child Widget
}

func (o constraintObserver) createBox(Ctx) layout.Box { return &observerBox{} }
func (o constraintObserver) updateBox(_ Ctx, b layout.Box) {
	b.(*observerBox).onCS = o.onCS
}
func (o constraintObserver) childWidgets() []Widget { return []Widget{o.Child} }
func (o constraintObserver) attach(b layout.Box, kids []layout.Box) {
	b.(*observerBox).Child = first(kids)
}

type observerBox struct {
	onCS  func(layout.Constraints)
	Child layout.Box
	size  geom.Size
}

func (b *observerBox) Layout(cs layout.Constraints) geom.Size {
	if b.onCS != nil {
		b.onCS(cs)
	}
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *observerBox) Size() geom.Size { return b.size }

func (b *observerBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *observerBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
}

func (b *observerBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}
