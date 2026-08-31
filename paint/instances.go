package paint

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
)

// MarkKind is the shape drawn for every mark in a Marks batch.
//
// The set is closed on purpose. A closed set can be rasterized by the CPU
// backend as a loop over a cached coverage stamp, which keeps headless
// rendering and the CPU fallback working — both load-bearing promises here.
// Handing an app a shader or a compute pass instead would be more powerful and
// would forfeit exactly that.
type MarkKind uint8

const (
	// MarkCircle is a filled circle of diameter Size, centred on the point.
	MarkCircle MarkKind = iota
	// MarkSquare is an axis-aligned filled square of side Size, centred.
	MarkSquare
)

func (k MarkKind) shape() gg.MarkShape {
	if k == MarkSquare {
		return gg.MarkSquare
	}
	return gg.MarkCircle
}

// Marks is a batch of identically-shaped marks drawn in one call.
//
// It exists because drawing N points as N separate fills spends nearly all of
// its time in the path rasterizer — building edges, flattening each mark's
// curves, sweeping scanlines — which is per-mark work whose answer does not
// depend on the mark. Profiling a 10,000-point scatter put ~75% of the frame
// there. A batch decides coverage once and then only places pixels.
//
// X and Y are required and must be the same length; that length is the number
// of marks. Size and Color are each either length 1 (one value for the whole
// batch) or the same length as X (one per mark). Any other combination draws
// nothing: a mismatched batch has no sensible reading, and guessing one would
// hide the caller's bug.
//
// Positions are logical pixels. Size is a diameter or side, not a radius.
//
// Pass the same *Marks across frames when the data has not changed — scene
// diffing compares batches by identity, exactly as it does images and paths,
// so a fresh value every frame forces a repaint.
type Marks struct {
	Kind  MarkKind
	X, Y  []float32
	Size  []float32
	Color []Color
}

// Len is the number of marks the batch draws, or 0 if it is malformed.
func (m *Marks) Len() int {
	n := len(m.X)
	if n == 0 || len(m.Y) != n {
		return 0
	}
	if len(m.Size) != 1 && len(m.Size) != n {
		return 0
	}
	if len(m.Color) != 1 && len(m.Color) != n {
		return 0
	}
	return n
}

// SizeAt returns the size for mark i, whether it is per-mark or per-batch.
func (m *Marks) SizeAt(i int) float32 {
	if len(m.Size) == 1 {
		return m.Size[0]
	}
	return m.Size[i]
}

// ColorAt is SizeAt for colour.
func (m *Marks) ColorAt(i int) Color {
	if len(m.Color) == 1 {
		return m.Color[0]
	}
	return m.Color[i]
}

// Bounds is the union of every mark's extent — what damage tracking and
// culling need. Empty for a malformed or empty batch.
func (m *Marks) Bounds() geom.Rect {
	n := m.Len()
	if n == 0 {
		return geom.Rect{}
	}
	var b geom.Rect
	for i := range n {
		h := m.SizeAt(i) / 2
		r := geom.Rect{
			Min: geom.Pt{X: m.X[i] - h, Y: m.Y[i] - h},
			Max: geom.Pt{X: m.X[i] + h, Y: m.Y[i] + h},
		}
		if i == 0 {
			b = r
			continue
		}
		b = b.Union(r)
	}
	return b
}

// drawMarks is the shared implementation behind Canvas.DrawMarks.
//
// The batch is offered to the direct blit first, which needs one diameter for
// the whole run. A per-mark Size is therefore grouped by size before being
// offered — worth it because "one size, many colours" is what a scatter
// actually looks like, and grouping keeps that on the fast path.
//
// Whatever the blit declines (a rotated transform, a rounded clip, an active
// GPU) falls back to filling each mark, which is what the caller would have
// written anyway. The fallback is a correctness guarantee, not a nicety: it is
// what lets the fast path stay narrow enough to be obviously right.
func drawMarks(c *ggCanvas, m *Marks) {
	n := m.Len()
	if n == 0 {
		return
	}
	if len(m.Size) == 1 {
		if blitMarkRun(c, m, m.Size[0], nil, n) {
			return
		}
		fillMarksSlow(c, m, nil, n)
		return
	}
	// Group by size, preserving order within each group.
	groups := map[float32][]int{}
	order := []float32{}
	for i := range n {
		s := m.Size[i]
		if _, seen := groups[s]; !seen {
			order = append(order, s)
		}
		groups[s] = append(groups[s], i)
	}
	for _, s := range order {
		idx := groups[s]
		if blitMarkRun(c, m, s, idx, len(idx)) {
			continue
		}
		fillMarksSlow(c, m, idx, len(idx))
	}
}

// blitMarkRun offers one same-size run to the direct blit. idx nil means "all
// marks, in order".
func blitMarkRun(c *ggCanvas, m *Marks, size float32, idx []int, n int) bool {
	if size <= 0 {
		return true // nothing to draw, and nothing for the fallback either
	}
	xs, ys := m.X, m.Y
	cols := make([]gg.RGBA, n)
	if idx == nil {
		for i := range n {
			cols[i] = m.ColorAt(i).ggRGBA()
		}
	} else {
		xs = make([]float32, n)
		ys = make([]float32, n)
		for k, i := range idx {
			xs[k], ys[k] = m.X[i], m.Y[i]
			cols[k] = m.ColorAt(i).ggRGBA()
		}
	}
	return c.dc.BlitMarks(m.Kind.shape(), float64(size), xs, ys, cols)
}

// fillMarksSlow draws each mark as its own shape.
func fillMarksSlow(c *ggCanvas, m *Marks, idx []int, n int) {
	for k := range n {
		i := k
		if idx != nil {
			i = idx[k]
		}
		s := m.SizeAt(i)
		if s <= 0 {
			continue
		}
		r := geom.RectXYWH(m.X[i]-s/2, m.Y[i]-s/2, s, s)
		radius := float32(0)
		if m.Kind == MarkCircle {
			radius = s / 2
		}
		c.FillRRect(r, radius, m.ColorAt(i))
	}
}
