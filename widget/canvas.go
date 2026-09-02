package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Canvas is the custom-painting escape hatch — a leaf that paints itself with
// Draw, the analog of Flutter's CustomPaint and the HTML <canvas>. Use it for
// graphics that don't decompose into widgets: charts, gauges, sparklines,
// generative art, game boards.
//
// Draw receives a paint.Canvas whose origin (0,0) is the surface's top-left,
// and the surface's logical size. The same primitives the rest of the
// framework paints with are available (fills, text, images, gradients, clips,
// transforms); on the GPU build they run through the GPU rasterizer.
//
// Sizing: by default the surface fills the space its parent offers; set W
// and/or H to request a fixed extent on that axis (0 = fill when the parent
// bounds it, else collapse). Drawing may overflow the surface (like Flutter's
// default) — set Clip to bound it. For pointer input, wrap in Interactive.
//
// Repainting: drawing is retained and re-recorded only when a frame is
// produced. To animate, drive frames from an enclosing Stateful (SetState) or
// a Ticker and read that state inside Draw.
type Canvas struct {
	W, H float32
	Clip bool // clip Draw to the surface bounds (default: no clip)
	Draw func(c paint.Canvas, size geom.Size)
}

func (cw Canvas) createBox(Ctx) layout.Box { return &canvasBox{} }
func (cw Canvas) updateBox(_ Ctx, b layout.Box) {
	cb := b.(*canvasBox)
	cb.w, cb.h, cb.clip, cb.draw = cw.W, cw.H, cw.Clip, cw.Draw
}
func (cw Canvas) childWidgets() []Widget          { return nil }
func (cw Canvas) attach(layout.Box, []layout.Box) {}

type canvasBox struct {
	w, h float32
	clip bool
	draw func(c paint.Canvas, size geom.Size)
	size geom.Size
}

func (b *canvasBox) Layout(cs layout.Constraints) geom.Size {
	// A zero dimension fills the available space (bounded), so a Canvas can
	// be a full-width control strip.
	w, h := b.w, b.h
	if w == 0 && cs.BoundedW() {
		w = cs.Max.W
	}
	if h == 0 && cs.BoundedH() {
		h = cs.Max.H
	}
	b.size = cs.Constrain(geom.Size{W: w, H: h})
	return b.size
}

func (b *canvasBox) Size() geom.Size { return b.size }

// Paint translates so Draw works in local coordinates (0,0 = top-left) and,
// when Clip is set, bounds drawing to the surface. The clip is pushed in parent
// coordinates, before the transform, so it bounds the surface regardless of
// what Draw does.
func (b *canvasBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.draw == nil || b.size.W <= 0 || b.size.H <= 0 {
		return
	}
	if b.clip {
		c.PushClip(geom.Rect{Min: at, Max: at.Add(b.size.Pt())})
	}
	c.PushTransform(paint.Transform{TX: at.X, TY: at.Y})
	b.draw(c, b.size)
	c.PopTransform()
	if b.clip {
		c.PopClip()
	}
}

// InkBounds implements layout.InkBounder: an unclipped Canvas may draw
// anywhere (Draw is arbitrary user code), so containers must never cull it;
// with Clip set, painting is bounded to the surface rect.
func (b *canvasBox) InkBounds() geom.Rect {
	if b.clip {
		return geom.RectFromSize(b.size)
	}
	return geom.Unbounded
}

func (b *canvasBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}
