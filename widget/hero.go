package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
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

func lerpRect(a, b geom.Rect, t float32) geom.Rect {
	lf := geom.LerpFloat
	return geom.Rect{
		Min: geom.Pt{X: lf(a.Min.X, b.Min.X, t), Y: lf(a.Min.Y, b.Min.Y, t)},
		Max: geom.Pt{X: lf(a.Max.X, b.Max.X, t), Y: lf(a.Max.Y, b.Max.Y, t)},
	}
}
