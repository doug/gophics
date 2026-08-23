package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// Reorderable is a list whose rows can be dragged into a new order.
//
// Rows are a uniform size along the axis. That is a real restriction and a
// deliberate one: with variable extents, the target index depends on the sizes
// of the rows being crossed, which are not known until they are laid out — so a
// general version needs measured geometry threaded through the drag, and would
// still behave unpredictably when a row resizes mid-drag. Uniform rows cover
// the cases people reorder (playlists, todo items, settings) and make the
// mapping from finger to index exact.
type Reorderable struct {
	Count int
	// ItemExtent is each row's size along Axis, in logical pixels. Required.
	ItemExtent float32
	// Build returns the row at index i in the *data's* order. The list handles
	// showing it somewhere else while a drag is in flight.
	Build func(i int) Widget
	// OnReorder fires once on drop, with the data indices. It is not called
	// when the row lands where it started.
	OnReorder func(from, to int)
	// Axis is the list direction (default vertical).
	Axis layout.Axis
}

func (r Reorderable) CreateState() State { return &reorderState{from: -1} }

type reorderState struct {
	StateBase[Reorderable]
	from  int     // index being dragged, or -1
	delta float32 // drag distance along the axis
}

func (s *reorderState) Build(ctx Ctx) Widget {
	w := s.W()
	if w.Build == nil || w.Count <= 0 {
		return Column()
	}
	target := s.target()
	order := displayOrder(w.Count, s.from, target)

	rows := make([]Widget, 0, len(order))
	for _, i := range order {
		rows = append(rows, s.row(i))
	}
	// Rows fill the cross axis. A plain Column or Row centres its children, so
	// a list of differently-sized rows comes out ragged and centred — and the
	// drag maths, which assumes a row occupies the full cross extent, no longer
	// matches where the row is drawn.
	if w.Axis == layout.Horizontal {
		r := Row(rows...)
		r.CrossAlign = layout.CrossStretch
		return r
	}
	c := Column(rows...)
	c.CrossAlign = layout.CrossStretch
	return c
}

// row wraps one item with its drag handling. The dragged row is offset to
// follow the finger; the others sit in their shifted positions, which is what
// makes the gap open ahead of the drop.
func (s *reorderState) row(i int) Widget {
	w := s.W()
	child := w.Build(i)

	axis := DragVertical
	off := geom.Pt{Y: s.delta}
	if w.Axis == layout.Horizontal {
		axis = DragHorizontal
		off = geom.Pt{X: s.delta}
	}
	if i == s.from {
		child = dragOffset{dx: off.X, dy: off.Y, child: child}
	}

	return Interactive{
		Handler: Handler{
			DragAxis: axis,
			OnPress:  func(geom.Pt) { s.SetState(func() { s.from, s.delta = i, 0 }) },
			OnDrag: func(_, d geom.Pt) {
				step := d.Y
				if w.Axis == layout.Horizontal {
					step = d.X
				}
				s.SetState(func() { s.delta += step })
			},
			OnRelease:  func() { s.drop() },
			OnPressEnd: func() { s.drop() },
		},
		Child: child,
	}
}

// target is where the dragged row would land right now.
func (s *reorderState) target() int {
	w := s.W()
	return reorderTarget(s.from, s.delta, w.ItemExtent, w.Count)
}

// drop ends the drag, reporting the move if the row actually moved.
func (s *reorderState) drop() {
	if s.from < 0 {
		return
	}
	from, to := s.from, s.target()
	s.SetState(func() { s.from, s.delta = -1, 0 })
	if to != from {
		if f := s.W().OnReorder; f != nil {
			f(from, to)
		}
	}
}

// reorderTarget maps a drag to the index the row would land on.
//
// The row moves a whole position each time it has travelled a full row, and the
// half-extent rounding is what makes the swap happen as the dragged row's
// centre passes its neighbour's — dropping the threshold to a full extent makes
// the list feel stuck, and to zero makes it flicker between two positions.
func reorderTarget(from int, delta, extent float32, count int) int {
	if from < 0 || count <= 0 {
		return from
	}
	if extent <= 0 {
		return from // unmeasured rows: refuse to guess rather than jump to 0
	}
	steps := int((delta + sign(delta)*extent/2) / extent)
	to := from + steps
	if to < 0 {
		return 0
	}
	if to >= count {
		return count - 1
	}
	return to
}

func sign(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}

// displayOrder is the order rows appear in while a drag is in flight: the
// dragged row lifted out and reinserted at its target, everything else closing
// up behind it.
//
// Returned as data indices so Build is always called with the item's real
// index — the app should not have to know a drag is happening to render its
// own row.
func displayOrder(count, from, to int) []int {
	order := make([]int, 0, count)
	for i := range count {
		order = append(order, i)
	}
	if from < 0 || from >= count || to < 0 || to >= count || from == to {
		return order
	}
	moved := order[from]
	order = append(order[:from], order[from+1:]...)
	rest := append([]int{}, order[to:]...)
	order = append(order[:to], moved)
	return append(order, rest...)
}

// dragOffset translates the row being dragged so it follows the finger, while
// the rows around it stay in their shifted positions.
type dragOffset struct {
	dx, dy float32
	child  Widget
}

func (d dragOffset) createBox(Ctx) layout.Box { return &layout.Translated{} }
func (d dragOffset) updateBox(_ Ctx, box layout.Box) {
	t := box.(*layout.Translated)
	t.Dx, t.Dy = d.dx, d.dy
}
func (d dragOffset) childWidgets() []Widget { return []Widget{d.child} }
func (d dragOffset) attach(box layout.Box, kids []layout.Box) {
	box.(*layout.Translated).Child = first(kids)
}
