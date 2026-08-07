// Package geom provides the geometric primitives used throughout gophics:
// points, sizes, rectangles, rounded rectangles, edge insets, and 2D affine
// transforms.
//
// All values are float32 in logical pixels (see design/adr/0001-float32-geometry.md).
// Types are small immutable values; methods return new values.
package geom

import "math"

// Pt is a point or vector in 2D space.
type Pt struct {
	X, Y float32
}

func (p Pt) Add(q Pt) Pt      { return Pt{p.X + q.X, p.Y + q.Y} }
func (p Pt) Sub(q Pt) Pt      { return Pt{p.X - q.X, p.Y - q.Y} }
func (p Pt) Mul(s float32) Pt { return Pt{p.X * s, p.Y * s} }
func (p Pt) In(r Rect) bool   { return r.Contains(p) }

// Lerp linearly interpolates from p to q; t=0 yields p, t=1 yields q.
func (p Pt) Lerp(q Pt, t float32) Pt {
	return Pt{p.X + (q.X-p.X)*t, p.Y + (q.Y-p.Y)*t}
}

// Size is a width and height in logical pixels, non-negative by convention.
type Size struct {
	W, H float32
}

// Lerp linearly interpolates between sizes a and b.
func (a Size) Lerp(b Size, t float32) Size {
	return Size{W: a.W + (b.W-a.W)*t, H: a.H + (b.H-a.H)*t}
}

// Lerp linearly interpolates between rects a and b (corner-wise).
func (a Rect) Lerp(b Rect, t float32) Rect {
	return Rect{Min: a.Min.Lerp(b.Min, t), Max: a.Max.Lerp(b.Max, t)}
}

// Lerp linearly interpolates between insets a and b.
func (a Insets) Lerp(b Insets, t float32) Insets {
	return Insets{
		Top:    a.Top + (b.Top-a.Top)*t,
		Right:  a.Right + (b.Right-a.Right)*t,
		Bottom: a.Bottom + (b.Bottom-a.Bottom)*t,
		Left:   a.Left + (b.Left-a.Left)*t,
	}
}

// LerpFloat linearly interpolates between two float32s.
func LerpFloat(a, b, t float32) float32 { return a + (b-a)*t }

func (s Size) IsEmpty() bool { return s.W <= 0 || s.H <= 0 }
func (s Size) Pt() Pt        { return Pt{s.W, s.H} }

// Rect is an axis-aligned rectangle defined by two corners.
// A Rect with Max components <= Min components is empty.
type Rect struct {
	Min, Max Pt
}

// RectXYWH constructs a Rect from an origin and dimensions.
func RectXYWH(x, y, w, h float32) Rect {
	return Rect{Pt{x, y}, Pt{x + w, y + h}}
}

// RectFromSize constructs a Rect at the origin with the given size.
func RectFromSize(s Size) Rect { return Rect{Max: Pt{s.W, s.H}} }

func (r Rect) Dx() float32 { return r.Max.X - r.Min.X }
func (r Rect) Dy() float32 { return r.Max.Y - r.Min.Y }
func (r Rect) Size() Size  { return Size{r.Dx(), r.Dy()} }

func (r Rect) IsEmpty() bool { return r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y }

// Contains reports whether p is inside r. Points on the Min edges are inside;
// points on the Max edges are outside (half-open, matching image.Rectangle).
func (r Rect) Contains(p Pt) bool {
	return p.X >= r.Min.X && p.X < r.Max.X && p.Y >= r.Min.Y && p.Y < r.Max.Y
}

// Intersect returns the largest rectangle contained by both r and s.
// If they do not overlap, the result is an empty Rect.
func (r Rect) Intersect(s Rect) Rect {
	r.Min.X = max(r.Min.X, s.Min.X)
	r.Min.Y = max(r.Min.Y, s.Min.Y)
	r.Max.X = min(r.Max.X, s.Max.X)
	r.Max.Y = min(r.Max.Y, s.Max.Y)
	if r.IsEmpty() {
		return Rect{}
	}
	return r
}

// Union returns the smallest rectangle containing both r and s.
// An empty rectangle does not contribute.
func (r Rect) Union(s Rect) Rect {
	if r.IsEmpty() {
		return s
	}
	if s.IsEmpty() {
		return r
	}
	return Rect{
		Min: Pt{min(r.Min.X, s.Min.X), min(r.Min.Y, s.Min.Y)},
		Max: Pt{max(r.Max.X, s.Max.X), max(r.Max.Y, s.Max.Y)},
	}
}

func (r Rect) Translate(p Pt) Rect {
	return Rect{r.Min.Add(p), r.Max.Add(p)}
}

// Radius is an x/y corner radius pair (elliptical corners).
type Radius struct {
	X, Y float32
}

// RadiusCircular returns a circular corner radius.
func RadiusCircular(r float32) Radius { return Radius{r, r} }

// RRect is a rectangle with per-corner elliptical radii.
type RRect struct {
	Rect
	TL, TR, BR, BL Radius
}

// RRectUniform returns an RRect with the same circular radius on all corners.
func RRectUniform(r Rect, radius float32) RRect {
	c := RadiusCircular(radius)
	return RRect{Rect: r, TL: c, TR: c, BR: c, BL: c}
}

// Insets describes offsets from the four edges of a rectangle.
type Insets struct {
	Top, Right, Bottom, Left float32
}

// InsetsAll returns uniform insets on all four edges.
func InsetsAll(v float32) Insets { return Insets{v, v, v, v} }

// InsetsSymmetric returns insets with the given horizontal (left/right) and
// vertical (top/bottom) values.
func InsetsSymmetric(horizontal, vertical float32) Insets {
	return Insets{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

func (i Insets) Horizontal() float32 { return i.Left + i.Right }
func (i Insets) Vertical() float32   { return i.Top + i.Bottom }

// Inset shrinks r by the insets. If the insets exceed the rectangle's size,
// the result collapses to a zero-size Rect at its center.
func (i Insets) Inset(r Rect) Rect {
	out := Rect{
		Min: Pt{r.Min.X + i.Left, r.Min.Y + i.Top},
		Max: Pt{r.Max.X - i.Right, r.Max.Y - i.Bottom},
	}
	if out.Min.X > out.Max.X {
		c := (out.Min.X + out.Max.X) / 2
		out.Min.X, out.Max.X = c, c
	}
	if out.Min.Y > out.Max.Y {
		c := (out.Min.Y + out.Max.Y) / 2
		out.Min.Y, out.Max.Y = c, c
	}
	return out
}

// Affine is a 2D affine transform:
//
//	| A C Tx |
//	| B D Ty |
//
// mapping (x, y) to (A*x + C*y + Tx, B*x + D*y + Ty).
type Affine struct {
	A, B, C, D, Tx, Ty float32
}

// Identity returns the identity transform.
func Identity() Affine { return Affine{A: 1, D: 1} }

// Translate returns a transform that translates by p.
func Translate(p Pt) Affine { return Affine{A: 1, D: 1, Tx: p.X, Ty: p.Y} }

// Scale returns a transform that scales by sx, sy about the origin.
func Scale(sx, sy float32) Affine { return Affine{A: sx, D: sy} }

// Rotate returns a transform that rotates by radians about the origin,
// clockwise in gophics's y-down coordinate system.
func Rotate(radians float32) Affine {
	sin, cos := math.Sincos(float64(radians))
	s, c := float32(sin), float32(cos)
	return Affine{A: c, B: s, C: -s, D: c}
}

// Mul returns the composition m∘n: applying the result is equivalent to
// applying n first, then m.
func (m Affine) Mul(n Affine) Affine {
	return Affine{
		A:  m.A*n.A + m.C*n.B,
		B:  m.B*n.A + m.D*n.B,
		C:  m.A*n.C + m.C*n.D,
		D:  m.B*n.C + m.D*n.D,
		Tx: m.A*n.Tx + m.C*n.Ty + m.Tx,
		Ty: m.B*n.Tx + m.D*n.Ty + m.Ty,
	}
}

// Apply transforms the point p.
func (m Affine) Apply(p Pt) Pt {
	return Pt{
		X: m.A*p.X + m.C*p.Y + m.Tx,
		Y: m.B*p.X + m.D*p.Y + m.Ty,
	}
}

// Invert returns the inverse transform and whether it exists (the transform
// is not degenerate).
func (m Affine) Invert() (Affine, bool) {
	det := m.A*m.D - m.B*m.C
	if det == 0 || float32(math.Abs(float64(det))) < 1e-12 {
		return Affine{}, false
	}
	inv := 1 / det
	return Affine{
		A:  m.D * inv,
		B:  -m.B * inv,
		C:  -m.C * inv,
		D:  m.A * inv,
		Tx: (m.C*m.Ty - m.D*m.Tx) * inv,
		Ty: (m.B*m.Tx - m.A*m.Ty) * inv,
	}, true
}
