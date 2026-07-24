package layout

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Axis is a layout direction. The zero value is Vertical: the natural
// default for scrolling and for a zero-value Flex (a column).
type Axis uint8

const (
	Vertical Axis = iota
	Horizontal
)

// MainAlign distributes free space along the main axis when no child flexes.
type MainAlign uint8

const (
	MainStart MainAlign = iota
	MainCenter
	MainEnd
	MainSpaceBetween
)

// CrossAlign positions children across the main axis.
type CrossAlign uint8

const (
	CrossStart CrossAlign = iota
	CrossCenter
	CrossEnd
	CrossStretch
)

// FlexChild is one flex participant. Flex 0 children get their intrinsic
// size; Flex > 0 children split the remaining main-axis space by weight.
type FlexChild struct {
	Box  Box
	Flex int
}

// Flexible wraps b with a flex weight.
func Flexible(flex int, b Box) FlexChild { return FlexChild{Box: b, Flex: flex} }

// Flex lays out children along an axis: Flutter's Row/Column.
//
// Layout: fixed children are measured with the main axis unbounded and the
// cross axis loosened (or tightened when CrossStretch). Remaining bounded
// main-axis space is split among flexed children by weight, tightly. The
// flex extent fills bounded main constraints and shrink-wraps unbounded ones.
type Flex struct {
	Base
	Axis       Axis
	MainAlign  MainAlign
	CrossAlign CrossAlign
	Children   []FlexChild
	offsets    []geom.Pt
}

// Row returns a horizontal Flex over plain children.
func Row(children ...Box) *Flex { return newFlex(Horizontal, children) }

// Column returns a vertical Flex over plain children.
func Column(children ...Box) *Flex { return newFlex(Vertical, children) }

func newFlex(axis Axis, children []Box) *Flex {
	f := &Flex{Axis: axis, CrossAlign: CrossCenter}
	for _, c := range children {
		f.Children = append(f.Children, FlexChild{Box: c})
	}
	return f
}

func (f *Flex) main(s geom.Size) float32 {
	if f.Axis == Horizontal {
		return s.W
	}
	return s.H
}

func (f *Flex) cross(s geom.Size) float32 {
	if f.Axis == Horizontal {
		return s.H
	}
	return s.W
}

func (f *Flex) size(main, cross float32) geom.Size {
	if f.Axis == Horizontal {
		return geom.Size{W: main, H: cross}
	}
	return geom.Size{W: cross, H: main}
}

func (f *Flex) pt(main, cross float32) geom.Pt {
	if f.Axis == Horizontal {
		return geom.Pt{X: main, Y: cross}
	}
	return geom.Pt{X: cross, Y: main}
}

func (f *Flex) Layout(cs Constraints) geom.Size {
	maxMain := f.main(cs.Max)
	maxCross := f.cross(cs.Max)
	boundedMain := maxMain != Inf

	childCross := Constraints{Max: f.size(Inf, maxCross)}
	if f.CrossAlign == CrossStretch && maxCross != Inf {
		childCross.Min = f.size(0, maxCross)
	}

	// Pass 1: fixed children.
	var usedMain, maxChildCross float32
	totalFlex := 0
	for _, c := range f.Children {
		if c.Flex > 0 {
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
	case MainCenter:
		pos = free / 2
	case MainEnd:
		pos = free
	case MainSpaceBetween:
		if n := len(f.Children); n > 1 {
			gap = free / float32(n-1)
		}
	}
	f.offsets = f.offsets[:0]
	for _, c := range f.Children {
		s := c.Box.Size()
		var crossPos float32
		switch f.CrossAlign {
		case CrossCenter:
			crossPos = (ownCross - f.cross(s)) / 2
		case CrossEnd:
			crossPos = ownCross - f.cross(s)
		}
		f.offsets = append(f.offsets, f.pt(pos, crossPos))
		pos += f.main(s) + gap
	}
	return f.setSize(own)
}

func (f *Flex) Paint(c *paint.Canvas, at geom.Pt) {
	for i, ch := range f.Children {
		ch.Box.Paint(c, at.Add(f.offsets[i]))
	}
}

func (f *Flex) AddHits(p geom.Pt, hits *[]Hit) {
	if !f.contains(p) {
		return
	}
	// Reverse order: later children paint on top.
	for i := len(f.Children) - 1; i >= 0; i-- {
		f.Children[i].Box.AddHits(p.Sub(f.offsets[i]), hits)
	}
	*hits = append(*hits, Hit{f, p})
}
