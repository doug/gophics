package layoutbox

import (
	"github.com/doug/gophics/layout"
	"slices"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// AspectRatio sizes its child to Ratio (width/height), as large as the
// constraints allow. Useful for media tiles and video frames.
type AspectRatio struct {
	Base
	Ratio float32 // width / height; <= 0 means 1
	Child layout.Box
}

func (b *AspectRatio) ratio() float32 {
	if b.Ratio <= 0 {
		return 1
	}
	return b.Ratio
}

func (b *AspectRatio) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	// Prefer the constrained width, derive height from the ratio; clamp.
	w := cs.Max.W
	if !cs.BoundedW() {
		w = cs.Min.W
	}
	h := w / b.ratio()
	if cs.BoundedH() && h > cs.Max.H {
		h = cs.Max.H
		w = h * b.ratio()
	}
	size := cs.Constrain(geom.Size{W: w, H: h})
	if b.Child != nil {
		b.Child.Layout(layout.Tight(size))
	}
	return b.Done(cs, size)
}

func (b *AspectRatio) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *AspectRatio) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *AspectRatio) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

// Filled expands to fill the largest size its constraints allow and paints
// an optional background color behind its child — the primitive for scrims,
// page backgrounds, and spacers-with-color.
type Filled struct {
	Base
	Color paint.Color
	Child layout.Box
}

func (b *Filled) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	size := geom.Size{W: cs.Min.W, H: cs.Min.H}
	if cs.BoundedW() {
		size.W = cs.Max.W
	}
	if cs.BoundedH() {
		size.H = cs.Max.H
	}
	size = cs.Constrain(size)
	if b.Child != nil {
		b.Child.Layout(layout.Tight(size))
	}
	return b.Done(cs, size)
}

func (b *Filled) Paint(c paint.Canvas, at geom.Pt) {
	if b.Color.A > 0 {
		c.FillRect(geom.Rect{Min: at, Max: at.Add(b.Size().Pt())}, b.Color)
	}
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *Filled) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *Filled) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

// Opacity composites its child as a group at Alpha [0,1] — the whole
// subtree fades together, not shape-by-shape (so overlapping content
// doesn't double-blend). Alpha 1 is a pass-through; 0 hides the child but
// keeps its layout size.
type Opacity struct {
	Base
	Alpha float32
	Child layout.Box
}

func (b *Opacity) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	if b.Child != nil {
		return b.Done(cs, b.Child.Layout(cs))
	}
	return b.Done(cs, cs.Constrain(geom.Size{}))
}

func (b *Opacity) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child == nil {
		return
	}
	a := b.Alpha
	if a >= 1 {
		b.Child.Paint(c, at) // opaque: skip the layer
		return
	}
	if a <= 0 {
		return // fully transparent: draw nothing
	}
	c.PushOpacity(a)
	b.Child.Paint(c, at)
	c.PopOpacity()
}

func (b *Opacity) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Alpha <= 0 {
		return // transparent groups don't receive input
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
}

func (b *Opacity) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

// Grid lays children out in a fixed number of equal-width columns, top to
// bottom, with uniform spacing. Row heights are the tallest cell in the row.
// It shrink-wraps its height to the content (wrap it in Scroll for long
// grids).
type Grid struct {
	Base
	Columns  int
	Spacing  float32
	Children []layout.Box
	offsets  []geom.Pt
}

func (b *Grid) cols() int {
	if b.Columns < 1 {
		return 1
	}
	return b.Columns
}

func (b *Grid) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	cols := b.cols()
	width := cs.Max.W
	if !cs.BoundedW() {
		width = cs.Min.W
	}
	cellW := (width - b.Spacing*float32(cols-1)) / float32(cols)
	if cellW < 0 {
		cellW = 0
	}
	cellCS := layout.Constraints{Max: geom.Size{W: cellW, H: layout.Inf}}

	b.offsets = b.offsets[:0]
	var x, y, rowH float32
	col := 0
	for _, ch := range b.Children {
		s := ch.Layout(cellCS)
		b.offsets = append(b.offsets, geom.Pt{X: x, Y: y})
		if s.H > rowH {
			rowH = s.H
		}
		col++
		if col == cols {
			col = 0
			x = 0
			y += rowH + b.Spacing
			rowH = 0
		} else {
			x += cellW + b.Spacing
		}
	}
	if col != 0 {
		y += rowH // last partial row
	} else if len(b.Children) > 0 {
		y -= b.Spacing // trailing gap from the last full row
	}
	return b.Done(cs, cs.Constrain(geom.Size{W: width, H: y}))
}

func (b *Grid) Paint(c paint.Canvas, at geom.Pt) {
	// Viewport culling, as in Flex.Paint: skip children whose ink lies
	// entirely outside the current clip, so a scrolled grid records only its
	// on-screen cells. ClipBounds is geom.Unbounded when unclipped or under a
	// transform, so nothing is dropped there.
	clip := c.ClipBounds()
	for i, ch := range b.Children {
		if i >= len(b.offsets) {
			break // children changed since last layout; skip until relaid
		}
		pos := at.Add(b.offsets[i])
		if !layout.InkBounds(ch).Translate(pos).Overlaps(clip) {
			continue
		}
		ch.Paint(c, pos)
	}
}

func (b *Grid) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	for i, v := range slices.Backward(b.Children) {
		if i < len(b.offsets) {
			v.AddHits(p.Sub(b.offsets[i]), hits)
		}
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *Grid) VisitChildren(visit func(layout.Box, geom.Pt)) {
	for i, ch := range b.Children {
		if i < len(b.offsets) {
			visit(ch, b.offsets[i])
		}
	}
}

// Wrap flows children along the main axis (layout.Horizontal by default),
// wrapping to a new run when the next child would overflow. Runs are packed
// along the cross axis. Spacing separates children; RunSpacing separates
// runs. Shrink-wraps the cross axis.
type Wrap struct {
	Base
	Spacing    float32
	RunSpacing float32
	Children   []layout.Box
	offsets    []geom.Pt
}

func (b *Wrap) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	maxW := cs.Max.W
	if !cs.BoundedW() {
		maxW = layout.Inf // the unbounded sentinel, so a child's BoundedW() stays false
	}
	childCS := layout.Constraints{Max: geom.Size{W: maxW, H: layout.Inf}}

	b.offsets = b.offsets[:0]
	var x, y, rowH, widest float32
	first := true
	for _, ch := range b.Children {
		s := ch.Layout(childCS)
		if !first && x+s.W > maxW {
			x = 0
			y += rowH + b.RunSpacing
			rowH = 0
		}
		b.offsets = append(b.offsets, geom.Pt{X: x, Y: y})
		x += s.W + b.Spacing
		if x-b.Spacing > widest {
			widest = x - b.Spacing
		}
		if s.H > rowH {
			rowH = s.H
		}
		first = false
	}
	total := y + rowH
	return b.Done(cs, cs.Constrain(geom.Size{W: widest, H: total}))
}

func (b *Wrap) Paint(c paint.Canvas, at geom.Pt) {
	// Viewport culling, as in Flex.Paint: skip children whose ink lies
	// entirely outside the current clip. ClipBounds is geom.Unbounded when
	// unclipped or under a transform, so nothing is dropped there.
	clip := c.ClipBounds()
	for i, ch := range b.Children {
		if i >= len(b.offsets) {
			break // children changed since last layout; skip until relaid
		}
		pos := at.Add(b.offsets[i])
		if !layout.InkBounds(ch).Translate(pos).Overlaps(clip) {
			continue
		}
		ch.Paint(c, pos)
	}
}

func (b *Wrap) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !b.contains(p) {
		return
	}
	for i, v := range slices.Backward(b.Children) {
		if i < len(b.offsets) {
			v.AddHits(p.Sub(b.offsets[i]), hits)
		}
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *Wrap) VisitChildren(visit func(layout.Box, geom.Pt)) {
	for i, ch := range b.Children {
		if i < len(b.offsets) {
			visit(ch, b.offsets[i])
		}
	}
}
