// Package layout is gophics's render-layer protocol: the Box interface, the
// constraints that flow through it, hit testing, and semantics. It is Flutter's
// rendering/ analog, reduced to the contract.
//
// The protocol is Flutter's, kept exactly: constraints flow down, sizes flow
// up, the parent positions the child. Layout is a single pass; a Box must
// return a size that satisfies the constraints it was given.
//
// Boxes are mutable render objects, configured and retained by the widget layer
// (or built by hand in tests). They are not thread-safe; all access is from the
// UI goroutine.
//
// The concrete boxes — Flex, Grid, Padded, Aligned and the rest — live in
// internal/layoutbox. They were here, which put sixteen implementations beside
// the protocol that describes them and shadowed the widget layer name for name:
// layout.Aligned against widget.Align, layout.Padded against widget.Padding.
// No app named one. Across every example in the module, this package is used
// for eleven names, all of them protocol, enums, or semantics.
//
// What stays here is what a custom widget or an embedding host actually needs:
// implement Box, take Constraints, return a size, answer AddHits and
// Semantics.
package layout

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Inf is the unbounded constraint value. It is a var only because Go has no
// way to write an untyped float32 +Inf constant. Do not mutate.
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

// InkBounder is an optional Box interface for boxes whose painting can
// extend beyond their layout rect ({0,0}–Size): Translated paints its child
// at an offset, Transformed scales/rotates it, Stack children can exceed the
// constrained size, and an unclipped widget.Canvas can draw anywhere.
// InkBounds returns the box's ink (paint) extent in its own coordinate
// space. Containers use it instead of the layout rect when viewport-culling,
// so such content is never wrongly dropped; return geom.Unbounded to opt out
// of culling entirely.
type InkBounder interface {
	InkBounds() geom.Rect
}

// InkBounds returns b's ink extent in its own coordinate space: the box's
// InkBounds if it implements InkBounder, else its layout rect.
func InkBounds(b Box) geom.Rect {
	if ib, ok := b.(InkBounder); ok {
		return ib.InkBounds()
	}
	return geom.RectFromSize(b.Size())
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

// clamp limits v to [lo, hi]. Kept alongside layoutbox's copy: three lines, and
// a shared internal helper package for it would be worse than the duplication.
func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
