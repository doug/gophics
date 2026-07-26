package widget

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// Hero marks a widget as a shared element for route transitions. When a push
// or pop moves between two routes that each contain a Hero with the same Tag,
// the element flies from its position on one route to its position on the
// other while the pages transition (Flutter's Hero). Wrap the same logical
// element — an image, an avatar, a title — with a matching Tag on both routes.
//
// The flight rebuilds Child in an overlay, so keep hero children cheap and
// stateless (images, text, icons).
type Hero struct {
	Tag   string
	Child Widget
}

func (h Hero) createBox(Ctx) layout.Box { return &heroBox{} }
func (h Hero) updateBox(ctx Ctx, b layout.Box) {
	hb := b.(*heroBox)
	hb.tag, hb.child = h.Tag, h.Child
	hb.reg, _ = Of[*heroRegistry](ctx) // nil outside a transition
}
func (h Hero) childWidgets() []Widget { return []Widget{h.Child} }
func (h Hero) attach(b layout.Box, kids []layout.Box) {
	b.(*heroBox).childBox = first(kids)
}

// heroRegistry is shared with a transitioning page (via Provide) so its heroes
// can report their painted rects and be suppressed while their flight runs.
type heroRegistry struct {
	rects  map[string]geom.Rect
	child  map[string]Widget
	flying map[string]bool
	frac   float32 // main-axis translate fraction applied to this page
}

func newHeroRegistry() *heroRegistry {
	return &heroRegistry{
		rects:  map[string]geom.Rect{},
		child:  map[string]Widget{},
		flying: map[string]bool{},
	}
}

// restRect removes a page's transition translate to recover a hero's at-rest
// rect (pages slide horizontally by frac × width).
func restRect(rc geom.Rect, frac, width float32) geom.Rect {
	return rc.Translate(geom.Pt{X: -frac * width})
}

type heroBox struct {
	layout.Base
	tag      string
	reg      *heroRegistry
	child    Widget
	childBox layout.Box
	size     geom.Size
}

func (b *heroBox) Layout(cs layout.Constraints) geom.Size {
	if b.childBox != nil {
		b.size = b.childBox.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *heroBox) Size() geom.Size { return b.size }

func (b *heroBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reg != nil && b.tag != "" {
		b.reg.rects[b.tag] = geom.Rect{Min: at, Max: at.Add(b.size.Pt())}
		b.reg.child[b.tag] = b.child
		if b.reg.flying[b.tag] {
			return // the flight overlay draws this element instead
		}
	}
	if b.childBox != nil {
		b.childBox.Paint(c, at)
	}
}

func (b *heroBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.childBox != nil {
		b.childBox.AddHits(p, hits)
	}
}

func (b *heroBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.childBox != nil {
		visit(b.childBox, geom.Pt{})
	}
}

// heroPageW translates a transitioning page horizontally (like translatedW)
// and records the applied fraction into its hero registry during paint — so
// the fraction always matches the rects the page's heroes captured that same
// paint, letting restRect recover their at-rest positions without frame lag.
type heroPageW struct {
	fracX float32
	reg   *heroRegistry
	child Widget
}

func (w heroPageW) createBox(Ctx) layout.Box { return &heroPageBox{} }
func (w heroPageW) updateBox(_ Ctx, b layout.Box) {
	hb := b.(*heroPageBox)
	hb.fracX, hb.reg = w.fracX, w.reg
}
func (w heroPageW) childWidgets() []Widget { return []Widget{w.child} }
func (w heroPageW) attach(b layout.Box, kids []layout.Box) {
	b.(*heroPageBox).child = first(kids)
}

type heroPageBox struct {
	layout.Base
	fracX float32
	reg   *heroRegistry
	child layout.Box
	size  geom.Size
}

func (b *heroPageBox) offset() geom.Pt { return geom.Pt{X: b.fracX * b.size.W} }

func (b *heroPageBox) Layout(cs layout.Constraints) geom.Size {
	if b.child != nil {
		b.size = b.child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *heroPageBox) Size() geom.Size { return b.size }

func (b *heroPageBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reg != nil {
		b.reg.frac = b.fracX // matches the rects captured below this paint
	}
	if b.child != nil {
		b.child.Paint(c, at.Add(b.offset()))
	}
}

func (b *heroPageBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.child != nil {
		b.child.AddHits(p.Sub(b.offset()), hits)
	}
}

func (b *heroPageBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.child != nil {
		visit(b.child, b.offset())
	}
}

func lerpRect(a, b geom.Rect, t float32) geom.Rect {
	lf := geom.LerpFloat
	return geom.Rect{
		Min: geom.Pt{X: lf(a.Min.X, b.Min.X, t), Y: lf(a.Min.Y, b.Min.Y, t)},
		Max: geom.Pt{X: lf(a.Max.X, b.Max.X, t), Y: lf(a.Max.Y, b.Max.Y, t)},
	}
}
