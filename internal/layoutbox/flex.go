package layoutbox

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// FlexChild is one flex participant. Flex 0 children get their intrinsic
// size; Flex > 0 children split the remaining main-axis space by weight.
type FlexChild struct {
	Box  layout.Box
	Flex int
}

// Flexible wraps b with a flex weight.
func Flexible(flex int, b layout.Box) FlexChild { return FlexChild{Box: b, Flex: flex} }

// Flex lays out children along an axis: Flutter's Row/Column.
//
// Layout: fixed children are measured with the main axis unbounded and the
// cross axis loosened (or tightened when layout.CrossStretch). Remaining bounded
// main-axis space is split among flexed children by weight, tightly. The
// flex extent fills bounded main constraints and shrink-wraps unbounded ones.
type Flex struct {
	Base
	Axis       layout.Axis
	MainAlign  layout.MainAlign
	CrossAlign layout.CrossAlign
	// Reverse runs the main axis the other way: the first child is placed at
	// the far end. Children keep their given order — only the direction of
	// travel flips — so layout.MainStart still means "packed against the start", it
	// is just that the start is now the right (or bottom) edge. This is what
	// mirrors a row for a right-to-left reading direction.
	Reverse  bool
	Children []FlexChild
	offsets  []geom.Pt
}

// Row returns a horizontal Flex over plain children.
func Row(children ...layout.Box) *Flex { return newFlex(layout.Horizontal, children) }

// Column returns a vertical Flex over plain children.
func Column(children ...layout.Box) *Flex { return newFlex(layout.Vertical, children) }

func newFlex(axis layout.Axis, children []layout.Box) *Flex {
	f := &Flex{Axis: axis, CrossAlign: layout.CrossCenter}
	for _, c := range children {
		f.Children = append(f.Children, FlexChild{Box: c})
	}
	return f
}

func (f *Flex) main(s geom.Size) float32 {
	if f.Axis == layout.Horizontal {
		return s.W
	}
	return s.H
}

func (f *Flex) cross(s geom.Size) float32 {
	if f.Axis == layout.Horizontal {
		return s.H
	}
	return s.W
}

func (f *Flex) size(main, cross float32) geom.Size {
	if f.Axis == layout.Horizontal {
		return geom.Size{W: main, H: cross}
	}
	return geom.Size{W: cross, H: main}
}

func (f *Flex) pt(main, cross float32) geom.Pt {
	if f.Axis == layout.Horizontal {
		return geom.Pt{X: main, Y: cross}
	}
	return geom.Pt{X: cross, Y: main}
}

func (f *Flex) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := f.Skip(cs); ok {
		return sz
	}
	maxMain := f.main(cs.Max)
	maxCross := f.cross(cs.Max)
	boundedMain := maxMain != layout.Inf

	childCross := layout.Constraints{Max: f.size(layout.Inf, maxCross)}
	if f.CrossAlign == layout.CrossStretch && maxCross != layout.Inf {
		childCross.Min = f.size(0, maxCross)
	}

	// Pass 1: fixed children — plus flex children when the main axis is unbounded
	// (they can't expand into an infinite remainder, so they size to their
	// content instead of being skipped and collapsing to zero).
	var usedMain, maxChildCross float32
	totalFlex := 0
	for _, c := range f.Children {
		if c.Flex > 0 && boundedMain {
			totalFlex += c.Flex
			continue
		}
		s := c.Box.Layout(childCross)
		usedMain += f.main(s)
		if cc := f.cross(s); cc > maxChildCross {
			maxChildCross = cc
		}
	}

	// Pass 2: flexed children split the remainder tightly.
	if totalFlex > 0 && boundedMain {
		remaining := max0(maxMain - usedMain)
		per := remaining / float32(totalFlex)
		for _, c := range f.Children {
			if c.Flex == 0 {
				continue
			}
			share := per * float32(c.Flex)
			ccs := childCross
			ccs.Min = f.size(share, f.cross(ccs.Min))
			ccs.Max = f.size(share, f.cross(ccs.Max))
			s := c.Box.Layout(ccs)
			usedMain += f.main(s)
			if cc := f.cross(s); cc > maxChildCross {
				maxChildCross = cc
			}
		}
	}

	mainSize := usedMain
	if boundedMain && totalFlex > 0 {
		mainSize = maxMain
	}
	own := cs.Constrain(f.size(mainSize, maxChildCross))
	ownMain, ownCross := f.main(own), f.cross(own)

	// Position children.
	free := max0(ownMain - usedMain)
	var pos, gap float32
	switch f.MainAlign {
	case layout.MainCenter:
		pos = free / 2
	case layout.MainEnd:
		pos = free
	case layout.MainSpaceBetween:
		if n := len(f.Children); n > 1 {
			gap = free / float32(n-1)
		}
	}
	f.offsets = f.offsets[:0]
	for _, c := range f.Children {
		s := c.Box.Size()
		var crossPos float32
		switch f.CrossAlign {
		case layout.CrossCenter:
			crossPos = (ownCross - f.cross(s)) / 2
		case layout.CrossEnd:
			crossPos = ownCross - f.cross(s)
		}
		f.offsets = append(f.offsets, f.pt(pos, crossPos))
		pos += f.main(s) + gap
	}
	if f.Reverse {
		// Mirror the main axis about the flex's own extent. Doing it after
		// placement rather than by walking the children backwards keeps every
		// other rule — flex distribution, layout.MainAlign, the gaps — computed once
		// and identically in both directions.
		for i := range f.offsets {
			s := f.Children[i].Box.Size()
			o := f.offsets[i]
			if f.Axis == layout.Horizontal {
				o.X = ownMain - o.X - s.W
			} else {
				o.Y = ownMain - o.Y - s.H
			}
			f.offsets[i] = o
		}
	}
	return f.Done(cs, own)
}

func (f *Flex) Paint(c paint.Canvas, at geom.Pt) {
	// Viewport culling: skip children lying entirely outside the current clip.
	// This makes scrolling a long list O(visible) instead of O(total) in the
	// record pass — e.g. a scrolled Column only records its on-screen rows.
	// The test uses ink bounds, not the layout rect, so children that paint
	// outside their layout box (Translated, Transformed, Stack, unclipped
	// Canvas — see layout.InkBounder) are never wrongly dropped. ClipBounds is
	// geom.Unbounded when unclipped or under a transform, so
	// unclipped/transformed content still paints in full (nothing is dropped).
	clip := c.ClipBounds()
	for i, ch := range f.Children {
		if i >= len(f.offsets) {
			break // children changed since last layout; skip until relaid
		}
		pos := at.Add(f.offsets[i])
		if !layout.InkBounds(ch.Box).Translate(pos).Overlaps(clip) {
			continue
		}
		ch.Box.Paint(c, pos)
	}
}

func (f *Flex) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if !f.contains(p) {
		return
	}
	// Reverse order: later children paint on top. Bounded by offsets in
	// case children changed since the last layout.
	n := min(len(f.Children), len(f.offsets))
	for i := n - 1; i >= 0; i-- {
		f.Children[i].Box.AddHits(p.Sub(f.offsets[i]), hits)
	}
	*hits = append(*hits, layout.Hit{Box: f, Pos: p})
}
