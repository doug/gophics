package widget

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// LazyList is a vertically scrolling list that mounts only the visible
// items (plus overscan): the M6 answer to thousand-item feeds. Item heights
// are measured as items appear and cached; unmeasured items use
// EstimatedExtent, so scrollbar math self-corrects as the user scrolls.
// Items are keyed by index, preserving their state while visible.
type LazyList struct {
	Count int
	// Build returns the widget for item i. Called only for items in or
	// near the viewport.
	Build func(i int) Widget
	// EstimatedExtent is the assumed height of unmeasured items (0 → 48).
	EstimatedExtent float32
	// OnEndReached fires when the list is scrolled near its end — grow
	// Count and rebuild for an infinite feed.
	OnEndReached func()
	// Controller exposes programmatic scrolling (jump/animate to offset).
	Controller *ScrollController
	// OnRefresh and Refreshing enable pull-to-refresh (see Scroll).
	OnRefresh  func()
	Refreshing bool
	// Reverse anchors the list to the end (newest at the bottom, pinned on
	// append) — the chat-log layout. OnEndReached then fires at the oldest
	// end, for loading history. See Scroll.Reverse.
	Reverse bool
}

func (l LazyList) estimate() float32 {
	if l.EstimatedExtent == 0 {
		return 48
	}
	return l.EstimatedExtent
}

func (l LazyList) CreateState() State { return &lazyState{} }

type lazyState struct {
	StateBase[LazyList]
	heights []float32 // measured heights; 0 = unmeasured
	offset  float32
	viewH   float32
}

func (s *lazyState) height(i int) float32 {
	if h := s.heights[i]; h > 0 {
		return h
	}
	return s.W().estimate()
}

func (s *lazyState) Build(Ctx) Widget {
	w := s.W()
	if len(s.heights) != w.Count {
		s.heights = make([]float32, w.Count)
	}
	viewH := s.viewH
	if viewH <= 0 {
		viewH = 800 // first frame: generous window until measured
	}
	overscan := viewH / 2
	// The scroll offset is measured from the start, or from the end when
	// reversed — map both to a content-space [winTop, winBottom] window.
	var winTop, winBottom float32
	if w.Reverse {
		var total float32
		for i := 0; i < w.Count; i++ {
			total += s.height(i)
		}
		winTop = total - s.offset - viewH - overscan
		winBottom = total - s.offset + overscan
	} else {
		winTop = s.offset - overscan
		winBottom = s.offset + viewH + overscan
	}

	children := make([]Widget, 0, 32)
	var y, topPad, bottomPad float32
	first := true
	for i := 0; i < w.Count; i++ {
		h := s.height(i)
		if y+h < winTop {
			topPad += h
		} else if y > winBottom {
			bottomPad += h
		} else {
			if first {
				children = append(children, Sized{H: topPad})
				first = false
			}
			children = append(children, WithKey{Key: i, Child: measured{Index: i, state: s, Child: w.Build(i)}})
		}
		y += h
	}
	if first {
		children = append(children, Sized{H: topPad})
	}
	children = append(children, Sized{H: bottomPad})

	col := Column(children...)
	col.CrossAlign = layout.CrossStretch
	return Scroll{
		Child:        col,
		Controller:   s.W().Controller,
		OnEndReached: s.W().OnEndReached,
		OnRefresh:    s.W().OnRefresh,
		Refreshing:   s.W().Refreshing,
		Reverse:      w.Reverse,
		OnOffset: func(off, extent float32) {
			s.SetState(func() { s.offset, s.viewH = off, extent })
		},
	}
}

// measured records its child's laid-out height into the list's cache.
type measured struct {
	Index int
	state *lazyState
	Child Widget
}

func (m measured) createBox(Ctx) layout.Box { return &measureBox{} }
func (m measured) updateBox(_ Ctx, b layout.Box) {
	mb := b.(*measureBox)
	mb.idx, mb.state = m.Index, m.state
}
func (m measured) childWidgets() []Widget { return []Widget{m.Child} }
func (m measured) attach(b layout.Box, kids []layout.Box) {
	b.(*measureBox).Child = first(kids)
}

type measureBox struct {
	idx   int
	state *lazyState
	Child layout.Box
	size  geom.Size
}

func (b *measureBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	if b.idx < len(b.state.heights) {
		b.state.heights[b.idx] = b.size.H
	}
	return b.size
}

func (b *measureBox) Size() geom.Size { return b.size }

func (b *measureBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *measureBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil && p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		b.Child.AddHits(p, hits)
	}
}

func (b *measureBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}
