// Package layoutbox holds the render-object implementations behind the widget
// layer: the concrete layout.Box types a widget builds, plus the inspector helpers.
//
// They were in package layout, which put sixteen box types beside the six-name
// protocol that describes them — and shadowed the widget layer name for name
// (layout.Aligned/widget.Align, layout.Padded/widget.Padding). No app named one:
// across every example, layout is used for exactly eleven protocol names and
// zero box types. So the protocol stays public in layout, where a custom widget
// or an embedder needs it, and the implementations live here.
package layoutbox

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"math"
	"slices"
)

// Base provides size storage and the layout skip-cache for layout.Box
// implementations: a clean box relayed the same constraints returns its
// cached size without recursing (the lightweight version of Flutter's
// relayout boundaries). Configuration changes must call MarkDirty —
// the widget layer does this for every updated box and its ancestors.
type Base struct {
	size   geom.Size
	lastCS layout.Constraints
	clean  bool
}

func (b *Base) Size() geom.Size { return b.size }

// Skip and Done are the layout-cache protocol, called ONLY by a box from
// inside its own Layout — Skip at the top (return the cached size early on a
// hit), Done at the end (record the result). They are exported so box
// implementations in other packages can embed Base and drive the cache; calling
// them from anywhere else (e.g. Done with a size the box did not lay out to)
// corrupts the cache and is a bug.

// Skip reports whether layout can be skipped for cs, returning the cached size.
func (b *Base) Skip(cs layout.Constraints) (geom.Size, bool) {
	if b.clean && cs == b.lastCS {
		return b.size, true
	}
	return geom.Size{}, false
}

// Done records the layout result for cs and marks the box clean.
func (b *Base) Done(cs layout.Constraints, s geom.Size) geom.Size {
	b.lastCS, b.size, b.clean = cs, s, true
	return s
}

// MarkDirty invalidates the skip-cache.
func (b *Base) MarkDirty() { b.clean = false }

// MarkDirty invalidates b's layout cache if it has one. Boxes without a
// cache always re-lay out, so this is safely a no-op for them.
func MarkDirty(b layout.Box) {
	if d, ok := b.(interface{ MarkDirty() }); ok {
		d.MarkDirty()
	}
}

func (b *Base) contains(p geom.Pt) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H
}

// Padded insets its child.
type Padded struct {
	Base
	Insets geom.Insets
	Child  layout.Box
}

func (b *Padded) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child == nil {
		return b.Done(cs, cs.Constrain(geom.Size{W: b.Insets.Horizontal(), H: b.Insets.Vertical()}))
	}
	child := b.Child.Layout(cs.Deflate(b.Insets))
	return b.Done(cs, cs.Constrain(geom.Size{
		W: child.W + b.Insets.Horizontal(),
		H: child.H + b.Insets.Vertical(),
	}))
}

func (b *Padded) childOffset() geom.Pt { return geom.Pt{X: b.Insets.Left, Y: b.Insets.Top} }

func (b *Padded) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at.Add(b.childOffset()))
	}
}

func (b *Padded) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p.Sub(b.childOffset()), hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

// Aligned positions its child within itself. AlignX/AlignY are in [0,1]:
// 0 = start (left/top), 0.5 = center, 1 = end. It expands to fill bounded
// constraints and shrink-wraps unbounded ones.
type Aligned struct {
	Base
	AlignX, AlignY float32
	Child          layout.Box
	offset         geom.Pt
}

// Center returns an Aligned that centers its child.
func Center(child layout.Box) *Aligned { return &Aligned{AlignX: 0.5, AlignY: 0.5, Child: child} }

func (b *Aligned) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	var child geom.Size
	if b.Child != nil {
		child = b.Child.Layout(cs.Loosen())
	}
	size := child
	if cs.BoundedW() {
		size.W = cs.Max.W
	}
	if cs.BoundedH() {
		size.H = cs.Max.H
	}
	size = cs.Constrain(size)
	b.offset = geom.Pt{
		X: (size.W - child.W) * b.AlignX,
		Y: (size.H - child.H) * b.AlignY,
	}
	return b.Done(cs, size)
}

func (b *Aligned) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at.Add(b.offset))
	}
}

func (b *Aligned) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p.Sub(b.offset), hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

// Sized forces specific dimensions on itself (and its child, if any).
// A zero dimension is "unspecified": the constraint passes through.
type Sized struct {
	Base
	W, H  float32
	Child layout.Box
}

func (b *Sized) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	inner := cs
	if b.W != 0 {
		w := clamp(b.W, cs.Min.W, cs.Max.W)
		inner.Min.W, inner.Max.W = w, w
	}
	if b.H != 0 {
		h := clamp(b.H, cs.Min.H, cs.Max.H)
		inner.Min.H, inner.Max.H = h, h
	}
	if b.Child != nil {
		return b.Done(cs, b.Child.Layout(inner))
	}
	return b.Done(cs, inner.Constrain(geom.Size{}))
}

func (b *Sized) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *Sized) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

// Translated shifts its child by (Dx + FracX·width, Dy) without affecting
// layout — the primitive under slide transitions.
type Translated struct {
	Base
	Dx, Dy float32
	// FracX adds a fraction of the box's own width (1 = fully offscreen
	// right), resolved at layout time.
	FracX float32
	// FracY does the same on the vertical axis. Together they place a box
	// relative to its own size, which is the only way to centre something on
	// a point when that size is not known until layout — a drag preview under
	// a finger, for one.
	FracY float32
	Child layout.Box
}

func (b *Translated) offsetPt() geom.Pt {
	return geom.Pt{X: b.Dx + b.FracX*b.Size().W, Y: b.Dy + b.FracY*b.Size().H}
}

func (b *Translated) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child == nil {
		return b.Done(cs, cs.Constrain(geom.Size{}))
	}
	return b.Done(cs, b.Child.Layout(cs))
}

func (b *Translated) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at.Add(b.offsetPt()))
	}
}

func (b *Translated) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil {
		b.Child.AddHits(p.Sub(b.offsetPt()), hits)
	}
}

// InkBounds implements layout.InkBounder: the child's ink shifted by the
// translation offset — where the child actually paints, not where it was
// laid out.
func (b *Translated) InkBounds() geom.Rect {
	if b.Child == nil {
		return geom.Rect{}
	}
	return layout.InkBounds(b.Child).Translate(b.offsetPt())
}

func (b *Translated) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, b.offsetPt())
	}
}

// Transformed applies an affine transform (paint.Transform) to its child
// when painting — scale, rotate, translate — sizing to the child unchanged
// (the transform is visual, like CSS transform). layout.Hit testing inverts the
// translate/scale so pointers still land (rotation is ignored in hit tests).
type Transformed struct {
	Base
	T paint.Transform
	// Center overrides the transform's pivot with the child's center (known
	// only at layout/paint time) — the natural origin for scale/rotate.
	Center bool
	Child  layout.Box
}

// pivotFor resolves the transform for this box at paint origin `at`: shifting
// the authored pivot into paint space, or using the child's center if Center.
func (b *Transformed) pivotFor(at geom.Pt) paint.Transform {
	t := b.T
	if b.Center {
		sz := b.Size()
		t.PivotX, t.PivotY = at.X+sz.W/2, at.Y+sz.H/2
	} else {
		t.PivotX += at.X
		t.PivotY += at.Y
	}
	return t
}

func (b *Transformed) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child == nil {
		return b.Done(cs, cs.Constrain(geom.Size{}))
	}
	return b.Done(cs, b.Child.Layout(cs))
}

func (b *Transformed) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child == nil {
		return
	}
	// The transform is authored in this box's local space; shift it to the
	// paint origin so a translate of (tx,ty) moves the child by that in local
	// coordinates regardless of where the box sits.
	c.PushTransform(b.pivotFor(at))
	b.Child.Paint(c, at)
	c.PopTransform()
}

func (b *Transformed) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child == nil {
		return
	}
	// Invert translate + scale about the pivot (rotation ignored). The pivot
	// is in this box's local space, so pass a zero origin.
	t := b.pivotFor(geom.Pt{})
	sx, sy := t.SX, t.SY
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	local := geom.Pt{
		X: t.PivotX + (p.X-t.TX-t.PivotX)/sx,
		Y: t.PivotY + (p.Y-t.TY-t.PivotY)/sy,
	}
	b.Child.AddHits(local, hits)
}

// InkBounds implements layout.InkBounder: the AABB of the child's ink mapped
// through the transform, in this box's coordinate space. The mapping mirrors
// the renderer (paint.Canvas.PushTransform): translate by (TX, TY), then
// scale and rotate about the pivot.
func (b *Transformed) InkBounds() geom.Rect {
	if b.Child == nil {
		return geom.Rect{}
	}
	ink := layout.InkBounds(b.Child)
	t := b.pivotFor(geom.Pt{}) // pivot in this box's local space (resolves Center)
	sx, sy := t.SX, t.SY
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	sin64, cos64 := math.Sincos(float64(t.Rotation))
	sin, cos := float32(sin64), float32(cos64)
	mapPt := func(p geom.Pt) geom.Pt {
		x, y := (p.X-t.PivotX)*sx, (p.Y-t.PivotY)*sy
		return geom.Pt{
			X: t.PivotX + t.TX + x*cos - y*sin,
			Y: t.PivotY + t.TY + x*sin + y*cos,
		}
	}
	c0 := mapPt(ink.Min)
	c1 := mapPt(geom.Pt{X: ink.Max.X, Y: ink.Min.Y})
	c2 := mapPt(geom.Pt{X: ink.Min.X, Y: ink.Max.Y})
	c3 := mapPt(ink.Max)
	return geom.Rect{
		Min: geom.Pt{X: min(c0.X, c1.X, c2.X, c3.X), Y: min(c0.Y, c1.Y, c2.Y, c3.Y)},
		Max: geom.Pt{X: max(c0.X, c1.X, c2.X, c3.X), Y: max(c0.Y, c1.Y, c2.Y, c3.Y)},
	}
}

func (b *Transformed) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{X: b.T.TX, Y: b.T.TY}) // best-effort (ignores scale)
	}
}

// Stack layers children atop each other (first is bottom), sized to the
// largest; the foundation for overlays and transitions.
type Stack struct {
	Base
	Children []layout.Box
	ink      geom.Rect // union of the children's ink, computed during Layout
}

func (b *Stack) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	var size geom.Size
	b.ink = geom.Rect{}
	for _, ch := range b.Children {
		s := ch.Layout(cs.Loosen())
		if s.W > size.W {
			size.W = s.W
		}
		if s.H > size.H {
			size.H = s.H
		}
		b.ink = b.ink.Union(layout.InkBounds(ch))
	}
	return b.Done(cs, cs.Constrain(size))
}

// InkBounds implements layout.InkBounder: the union of the children's ink as of the
// last Layout. Stack does not clip, and children may exceed its constrained
// size (over-constraint) or paint at offsets (Translated), so the union —
// not the layout rect — is what a culling ancestor must test.
func (b *Stack) InkBounds() geom.Rect { return b.ink }

func (b *Stack) Paint(c paint.Canvas, at geom.Pt) {
	// Viewport culling, as in Flex.Paint: skip children whose ink lies
	// entirely outside the current clip. ClipBounds is geom.Unbounded when
	// unclipped or under a transform, so nothing is dropped there.
	clip := c.ClipBounds()
	for _, ch := range b.Children {
		if !layout.InkBounds(ch).Translate(at).Overlaps(clip) {
			continue
		}
		ch.Paint(c, at)
	}
}

func (b *Stack) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	for _, v := range slices.Backward(b.Children) {
		v.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *Stack) VisitChildren(visit func(layout.Box, geom.Pt)) {
	for _, ch := range b.Children {
		visit(ch, geom.Pt{})
	}
}

// Decorated paints a rounded-rect background (and optional border) behind
// its child, sizing to the child (or to constraints if childless).
type Decorated struct {
	Base
	Color       paint.Color
	Radius      float32
	BorderColor paint.Color
	BorderWidth float32
	// Blur, when > 0, frosts the backdrop behind the surface (a glass material)
	// before the Color tint is painted over it. Pair with a translucent Color.
	Blur  float32
	Child layout.Box
}

func (b *Decorated) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child != nil {
		return b.Done(cs, b.Child.Layout(cs))
	}
	return b.Done(cs, cs.Constrain(geom.Size{}))
}

func (b *Decorated) Paint(c paint.Canvas, at geom.Pt) {
	r := geom.Rect{Min: at, Max: at.Add(b.Size().Pt())}
	if b.Blur > 0 {
		// Frost the backdrop within the rounded surface before tinting over it.
		c.PushClipRRect(r, b.Radius)
		c.BackdropBlur(r, b.Blur)
		c.PopClip()
	}
	if b.Color.A > 0 {
		c.FillRRect(r, b.Radius, b.Color)
	}
	if b.BorderWidth > 0 && b.BorderColor.A > 0 {
		c.StrokeRRect(r, b.Radius, b.BorderWidth, b.BorderColor)
	}
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *Decorated) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max0(v float32) float32 {
	if v < 0 {
		return 0
	}
	return v
}
