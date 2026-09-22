package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// LayoutObserver reports where its child ended up: the rect it occupies in
// root space, after any layout/paint pass in which its size or position
// changed. LayoutBuilder answers "what room do I have?" before layout; this
// answers "what did I get?" after it, which is what a drag board needs to
// know which lane a drop point fell in, or a tier list needs to size its
// rows to their measured contents instead of a fixed height.
//
// The rect's origin is the point Paint received for the child — root space,
// scroll offsets included. A Transform above it is not folded in (the origin
// is where the child was asked to paint, in the transformed canvas's own
// coordinates), so a scaled or rotated subtree reports the rect it would
// have had untransformed.
//
// OnLayout is delivered through the owner's Post, after the frame that
// measured it, never from inside layout or paint. It may call SetState
// freely; the rebuild lands on the following frame, one frame after the
// change it reacts to — the same latency LayoutBuilder has, and invisible
// for the same reason. It is not called again while the rect is unchanged.
//
// Because the rect is taken at paint, a child that is never painted never
// reports: a Viewport culls children wholly outside its visible region, so
// rows below the fold of a Scroll have no rect until they are scrolled into
// view. A layout that needs every row's size regardless — a tier list sizing
// itself to its tallest row — should measure with LayoutBuilder or a fixed
// height rather than wait on observers it cannot see.
type LayoutObserver struct {
	OnLayout func(rect geom.Rect)
	Child    Widget
}

func (o LayoutObserver) createBox(ctx Ctx) layout.Box {
	return &layoutObserverBox{onLayout: o.OnLayout, post: ctx.Post()}
}

func (o LayoutObserver) updateBox(ctx Ctx, b layout.Box) {
	lb := b.(*layoutObserverBox)
	lb.onLayout, lb.post = o.OnLayout, ctx.Post()
}

func (o LayoutObserver) childWidgets() []Widget { return []Widget{o.Child} }
func (o LayoutObserver) soleChild() Widget      { return o.Child }
func (o LayoutObserver) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutObserverBox).Child = first(kids)
}

type layoutObserverBox struct {
	onLayout func(geom.Rect)
	post     func(func())
	Child    layout.Box
	size     geom.Size

	// reported is the last rect handed to OnLayout, so an unchanged frame is
	// silent: a scene is re-painted far more often than it moves.
	reported     geom.Rect
	haveReported bool
}

func (b *layoutObserverBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *layoutObserverBox) Size() geom.Size { return b.size }

// Paint is where the origin becomes known — layout only learns sizes — which
// is why the report happens here rather than in Layout.
func (b *layoutObserverBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
	rect := geom.Rect{Min: at, Max: at.Add(b.size.Pt())}
	if b.onLayout == nil || (b.haveReported && rect == b.reported) {
		return
	}
	b.reported, b.haveReported = rect, true
	cb := b.onLayout
	if b.post == nil {
		// No runner (a bare Owner in a unit test): deliver directly. Paint is
		// past layout, so a SetState here only queues a rebuild.
		cb(rect)
		return
	}
	b.post(func() { cb(rect) })
}

func (b *layoutObserverBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
}

// InkBounds forwards the child's, so a child that paints outside its rect is
// not culled by a viewport just because this wrapper sits between them.
func (b *layoutObserverBox) InkBounds() geom.Rect {
	if b.Child == nil {
		return geom.RectFromSize(b.size)
	}
	return layout.InkBounds(b.Child)
}

func (b *layoutObserverBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}
