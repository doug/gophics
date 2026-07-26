package layout

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Viewport shows a scrolled window onto a child laid out with an unbounded
// main axis. Offset is clamped during layout to the scrollable range; read
// it back (or MaxOffset) after layout for scrollbar geometry.
type Viewport struct {
	Base
	Axis   Axis // Vertical: content scrolls up as Offset grows
	Offset float32
	// Lead is extra space revealed at the leading edge (top/left) by
	// translating content along the main axis without clamping — pull-to-
	// refresh draws its indicator in the revealed band. Unlike Offset it does
	// not participate in scroll-range clamping.
	Lead float32
	// Reverse anchors scrolling to the trailing edge: Offset 0 shows the end
	// (bottom/right) with content aligned there, and Offset grows toward the
	// start. Content shorter than the viewport sits at the end. This is the
	// chat-log layout — appended content stays pinned to the end for free,
	// since the offset origin is the end.
	Reverse bool
	Child   Box

	content geom.Size
}

// MaxOffset is the maximum scroll offset as of the last layout.
func (v *Viewport) MaxOffset() float32 {
	if v.Axis == Horizontal {
		return max0(v.content.W - v.Size().W)
	}
	return max0(v.content.H - v.Size().H)
}

func (v *Viewport) Layout(cs Constraints) geom.Size {
	if sz, ok := v.Skip(cs); ok {
		return sz
	}
	inner := cs.Loosen()
	if v.Axis == Horizontal {
		inner.Max.W = Inf
	} else {
		inner.Max.H = Inf
	}
	if v.Child != nil {
		v.content = v.Child.Layout(inner)
	} else {
		v.content = geom.Size{}
	}
	size := cs.Constrain(v.content)
	v.Done(cs, size)
	v.Offset = clamp(v.Offset, 0, v.MaxOffset())
	return size
}

func (v *Viewport) scrollPt() geom.Pt {
	if v.Axis == Horizontal {
		x := -v.Offset
		if v.Reverse {
			x = v.Size().W - v.content.W + v.Offset
		}
		return geom.Pt{X: x + v.Lead}
	}
	y := -v.Offset
	if v.Reverse {
		y = v.Size().H - v.content.H + v.Offset
	}
	return geom.Pt{Y: y + v.Lead}
}

func (v *Viewport) Paint(c paint.Canvas, at geom.Pt) {
	if v.Child == nil {
		return
	}
	c.PushClip(geom.Rect{Min: at, Max: at.Add(v.Size().Pt())})
	v.Child.Paint(c, at.Add(v.scrollPt()))
	c.PopClip()
}

func (v *Viewport) AddHits(p geom.Pt, hits *[]Hit) {
	if !v.contains(p) {
		return
	}
	if v.Child != nil {
		v.Child.AddHits(p.Sub(v.scrollPt()), hits)
	}
	*hits = append(*hits, Hit{v, p})
}
