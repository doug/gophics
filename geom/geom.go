// Package geom provides the geometric primitives used throughout gophics:
// points, sizes, rectangles, and edge insets.
//
// All values are float32 in logical pixels (see design/adr/0001-float32-geometry.md).
// Types are small immutable values; methods return new values.
package geom

// Pt is a point or vector in 2D space.
type Pt struct {
	X, Y float32
}

func (p Pt) Add(q Pt) Pt      { return Pt{p.X + q.X, p.Y + q.Y} }
func (p Pt) Sub(q Pt) Pt      { return Pt{p.X - q.X, p.Y - q.Y} }
func (p Pt) Mul(s float32) Pt { return Pt{p.X * s, p.Y * s} }

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
