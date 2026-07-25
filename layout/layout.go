// Package layout implements gossamer's render layer: the box protocol and
// the core layout boxes. It is Flutter's rendering/ analog (PLAN.md M2).
//
// The protocol is Flutter's, kept exactly (PLAN.md §4.5): constraints flow
// down, sizes flow up, the parent positions the child. Layout is a single
// pass; a Box must return a size that satisfies the constraints it was given.
//
// Boxes are mutable render objects, configured and retained by the widget
// layer (or built by hand in tests). They are not thread-safe; all access is
// from the UI goroutine.
package layout

import (
	"math"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Inf is the unbounded constraint value.
var Inf = float32(math.Inf(1))

// Constraints are immutable min/max bounds handed down during layout.
// A dimension with Max == Inf is unbounded.
type Constraints struct {
	Min, Max geom.Size
}

// Tight returns constraints that admit exactly s.
func Tight(s geom.Size) Constraints { return Constraints{Min: s, Max: s} }

// Loose returns constraints from zero up to s.
func Loose(s geom.Size) Constraints { return Constraints{Max: s} }

// Unbounded returns constraints with no maximum.
func Unbounded() Constraints { return Constraints{Max: geom.Size{W: Inf, H: Inf}} }

// Constrain returns the size nearest to s that satisfies c.
func (c Constraints) Constrain(s geom.Size) geom.Size {
	return geom.Size{
		W: clamp(s.W, c.Min.W, c.Max.W),
		H: clamp(s.H, c.Min.H, c.Max.H),
	}
}

// Loosen removes the minimums, keeping the maximums.
func (c Constraints) Loosen() Constraints { return Constraints{Max: c.Max} }

// Deflate shrinks the constraints by the given insets, for laying out a
// child inside padding.
func (c Constraints) Deflate(i geom.Insets) Constraints {
	h, v := i.Horizontal(), i.Vertical()
	return Constraints{
		Min: geom.Size{W: max0(c.Min.W - h), H: max0(c.Min.H - v)},
		Max: geom.Size{W: max0(c.Max.W - h), H: max0(c.Max.H - v)},
	}
}

func (c Constraints) BoundedW() bool { return !math.IsInf(float64(c.Max.W), 1) }
func (c Constraints) BoundedH() bool { return !math.IsInf(float64(c.Max.H), 1) }

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

// Box is a render object. Layout must be called before Paint or HitTest.
type Box interface {
	// Layout computes and stores the box's size, laying out children.
	// The returned size must satisfy cs.
	Layout(cs Constraints) geom.Size
	// Size returns the size computed by the last Layout.
	Size() geom.Size
	// Paint draws the box with its origin at 'at' in canvas coordinates.
	Paint(c paint.Canvas, at geom.Pt)
	// AddHits appends boxes containing p (in this box's local coordinates)
	// to hits, deepest (visually front-most) first.
	AddHits(p geom.Pt, hits *[]Hit)
}

// Hit is one box on a hit path, with p in that box's local coordinates.
type Hit struct {
	Box Box
	Pos geom.Pt
}

// HitTest returns the boxes under p, deepest first.
func HitTest(b Box, p geom.Pt) []Hit {
	var hits []Hit
	b.AddHits(p, &hits)
	return hits
}

// Base provides size storage and the layout skip-cache for Box
// implementations: a clean box relayed the same constraints returns its
// cached size without recursing (the lightweight version of Flutter's
// relayout boundaries). Configuration changes must call MarkLayoutDirty —
// the widget layer does this for every updated box and its ancestors.
type Base struct {
	size   geom.Size
	lastCS Constraints
	clean  bool
}

func (b *Base) Size() geom.Size { return b.size }

// Skip reports whether layout can be skipped for cs, returning the cached
// size. Boxes call it at the top of Layout.
func (b *Base) Skip(cs Constraints) (geom.Size, bool) {
	if b.clean && cs == b.lastCS {
		return b.size, true
	}
	return geom.Size{}, false
}

// Done records the layout result for cs and marks the box clean.
func (b *Base) Done(cs Constraints, s geom.Size) geom.Size {
	b.lastCS, b.size, b.clean = cs, s, true
	return s
}

// MarkLayoutDirty invalidates the skip-cache.
func (b *Base) MarkLayoutDirty() { b.clean = false }

// MarkDirty invalidates b's layout cache if it has one. Boxes without a
// cache always re-lay out, so this is safely a no-op for them.
func MarkDirty(b Box) {
	if d, ok := b.(interface{ MarkLayoutDirty() }); ok {
		d.MarkLayoutDirty()
	}
}

func (b *Base) contains(p geom.Pt) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H
}

// Padded insets its child.
type Padded struct {
	Base
	Insets geom.Insets
	Child  Box
}

func (b *Padded) Layout(cs Constraints) geom.Size {
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

func (b *Padded) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p.Sub(b.childOffset()), hits)
	}
	*hits = append(*hits, Hit{b, p})
}

// Aligned positions its child within itself. AlignX/AlignY are in [0,1]:
// 0 = start (left/top), 0.5 = center, 1 = end. It expands to fill bounded
// constraints and shrink-wraps unbounded ones.
type Aligned struct {
	Base
	AlignX, AlignY float32
	Child          Box
	offset         geom.Pt
}

// Center returns an Aligned that centers its child.
func Center(child Box) *Aligned { return &Aligned{AlignX: 0.5, AlignY: 0.5, Child: child} }

func (b *Aligned) Layout(cs Constraints) geom.Size {
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

func (b *Aligned) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p.Sub(b.offset), hits)
	}
	*hits = append(*hits, Hit{b, p})
}

// Sized forces specific dimensions on itself (and its child, if any).
// A zero dimension is "unspecified": the constraint passes through.
type Sized struct {
	Base
	W, H  float32
	Child Box
}

func (b *Sized) Layout(cs Constraints) geom.Size {
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

func (b *Sized) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, Hit{b, p})
}

// Decorated paints a rounded-rect background (and optional border) behind
// its child, sizing to the child (or to constraints if childless).
type Decorated struct {
	Base
	Color       paint.Color
	Radius      float32
	BorderColor paint.Color
	BorderWidth float32
	Child       Box
}

func (b *Decorated) Layout(cs Constraints) geom.Size {
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

func (b *Decorated) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, Hit{b, p})
}
