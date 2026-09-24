package theme

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// IconGlyph names one of the shapes Icon can draw.
type IconGlyph uint8

const (
	IconHome IconGlyph = iota
	IconList
	IconSearch
	IconSliders
	IconChevronLeft
	IconChevronRight
	IconChevronUp
	IconChevronDown
	IconPlus
	IconClose
	IconCheck
	IconMenu
	IconCalendar
	IconChart
	// Appended after the first set: the numeric order is API, so a new glyph
	// goes at the end whatever it would sit beside alphabetically.
	IconBell
	IconPeople
	IconPerson
	IconPuzzle
	// IconGrip is the ⋮⋮ drag handle — two dotted columns — for a row that
	// can be reordered.
	IconGrip
	IconReply
	IconCheckCircle
	IconTrash
	IconShare
	IconCopy
)

// Icon draws a small vector glyph — the shapes a nav bar, a toolbar and a table
// need — sized and coloured like the text beside it.
//
// These are paths rather than an icon font because a font can be missing the
// glyph. The Go fonts gophics recommends cover Latin, Cyrillic, Greek and a
// handful of symbols, and none of the geometric shapes a nav bar reaches for:
// of a set like ◎ ▤ ⇄ ◫ ◑ ◈ ↻ ◭ ≡, only ≡ resolves and the rest draw as tofu.
// That is not a rendering bug, but it is diagnosed as one, and it is met on
// roughly the first screen anyone builds. A path has no such failure mode: it
// scales with the type, needs no font registration, and cannot come out empty.
//
// The shapes are drawn on a 24x24 grid and stroked with round caps, so they sit
// beside text at the same optical weight at any size.
type Icon struct {
	Glyph IconGlyph
	// Size is the glyph's box in logical px; 0 takes the body text size, so an
	// icon matches the line it sits on.
	Size float32
	// Color is the stroke; zero takes the theme's text color.
	Color paint.Color
	// Stroke is the line width; 0 scales with Size, matching the type's weight.
	Stroke float32
}

func (ic Icon) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	size := ic.Size
	if size <= 0 {
		size = th.Type.Body
	}
	col := colorOr(ic.Color, th.Text)
	stroke := ic.Stroke
	if stroke <= 0 {
		stroke = size / 12 // 2px on a 24px icon
	}
	glyph := ic.Glyph
	return widget.Sized{W: size, H: size, Child: widget.Canvas{
		Draw: func(c paint.Canvas, box geom.Size) {
			s := min(box.W, box.H) / 24
			if s <= 0 {
				return
			}
			p := paint.NewPath()
			drawGlyph(p, glyph, s)
			c.StrokePath(p, stroke, col)
		},
	}}
}

// at scales a point from the 24x24 design grid.
func at(s, x, y float32) geom.Pt { return geom.Pt{X: x * s, Y: y * s} }

// arc appends a circle centred at (cx,cy) with radius r, as four cubics.
// 0.5523 is the standard circular approximation constant.
func arc(p *paint.Path, s, cx, cy, r float32) {
	const k = 0.5523
	p.MoveTo(at(s, cx+r, cy))
	p.CubeTo(at(s, cx+r, cy+r*k), at(s, cx+r*k, cy+r), at(s, cx, cy+r))
	p.CubeTo(at(s, cx-r*k, cy+r), at(s, cx-r, cy+r*k), at(s, cx-r, cy))
	p.CubeTo(at(s, cx-r, cy-r*k), at(s, cx-r*k, cy-r), at(s, cx, cy-r))
	p.CubeTo(at(s, cx+r*k, cy-r), at(s, cx+r, cy-r*k), at(s, cx+r, cy))
}

// line appends one segment.
func line(p *paint.Path, s, x1, y1, x2, y2 float32) {
	p.MoveTo(at(s, x1, y1))
	p.LineTo(at(s, x2, y2))
}

// chevron appends a two-segment arrow pointing in the given direction.
func iconChevron(p *paint.Path, s float32, g IconGlyph) {
	switch g {
	case IconChevronLeft:
		p.MoveTo(at(s, 15, 5))
		p.LineTo(at(s, 8, 12))
		p.LineTo(at(s, 15, 19))
	case IconChevronRight:
		p.MoveTo(at(s, 9, 5))
		p.LineTo(at(s, 16, 12))
		p.LineTo(at(s, 9, 19))
	case IconChevronUp:
		p.MoveTo(at(s, 5, 15))
		p.LineTo(at(s, 12, 8))
		p.LineTo(at(s, 19, 15))
	default: // IconChevronDown
		p.MoveTo(at(s, 5, 9))
		p.LineTo(at(s, 12, 16))
		p.LineTo(at(s, 19, 9))
	}
}

func drawGlyph(p *paint.Path, g IconGlyph, s float32) {
	switch g {
	case IconHome:
		p.MoveTo(at(s, 3, 11))
		p.LineTo(at(s, 12, 3))
		p.LineTo(at(s, 21, 11))
		p.MoveTo(at(s, 5.5, 10)) // walls, drawn from the eaves down
		p.LineTo(at(s, 5.5, 20))
		p.LineTo(at(s, 18.5, 20))
		p.LineTo(at(s, 18.5, 10))
	case IconList:
		for _, y := range []float32{6, 12, 18} {
			line(p, s, 8, y, 20, y)
			line(p, s, 4, y, 4.6, y) // bullet, a dot the round cap fills out
		}
	case IconSearch:
		arc(p, s, 11, 11, 6)
		line(p, s, 15.5, 15.5, 20, 20)
	case IconSliders:
		// Lines 6 apart with knobs 4 tall: any closer and a knob reads as
		// touching the line above it rather than riding its own.
		for _, r := range []struct{ y, knob float32 }{{6, 15}, {12, 9}, {18, 17}} {
			line(p, s, 4, r.y, 20, r.y)
			line(p, s, r.knob, r.y-2, r.knob, r.y+2)
		}
	case IconChevronLeft, IconChevronRight, IconChevronUp, IconChevronDown:
		iconChevron(p, s, g)
	case IconPlus:
		line(p, s, 12, 5, 12, 19)
		line(p, s, 5, 12, 19, 12)
	case IconClose:
		line(p, s, 6, 6, 18, 18)
		line(p, s, 18, 6, 6, 18)
	case IconCheck:
		p.MoveTo(at(s, 5, 13))
		p.LineTo(at(s, 10, 18))
		p.LineTo(at(s, 19, 6))
	case IconMenu:
		for _, y := range []float32{7, 12, 17} {
			line(p, s, 4, y, 20, y)
		}
	case IconCalendar:
		p.MoveTo(at(s, 4, 7))
		p.LineTo(at(s, 20, 7))
		p.LineTo(at(s, 20, 20))
		p.LineTo(at(s, 4, 20))
		p.Close()
		line(p, s, 4, 11, 20, 11)
		line(p, s, 8, 4, 8, 7)
		line(p, s, 16, 4, 16, 7)
	case IconChart:
		line(p, s, 4, 20, 20, 20)
		line(p, s, 7, 20, 7, 13)
		line(p, s, 12, 20, 12, 7)
		line(p, s, 17, 20, 17, 16)
	case IconBell:
		// The body is a dome on two short walls, closed across the rim; the
		// clapper is a small arc below it so the shape reads as a bell and
		// not as a padlock.
		p.MoveTo(at(s, 5, 17))
		p.LineTo(at(s, 6.5, 15))
		p.LineTo(at(s, 6.5, 11))
		p.CubeTo(at(s, 6.5, 7.5), at(s, 9, 5), at(s, 12, 5))
		p.CubeTo(at(s, 15, 5), at(s, 17.5, 7.5), at(s, 17.5, 11))
		p.LineTo(at(s, 17.5, 15))
		p.LineTo(at(s, 19, 17))
		p.Close()
		p.MoveTo(at(s, 10, 20))
		p.CubeTo(at(s, 10.5, 21), at(s, 11.2, 21.3), at(s, 12, 21.3))
		p.CubeTo(at(s, 12.8, 21.3), at(s, 13.5, 21), at(s, 14, 20))
	case IconPerson:
		arc(p, s, 12, 8, 3.5)
		shoulders(p, s, 12, 5, 19)
	case IconPeople:
		// One whole figure in front, and behind it the right half of a second
		// — a head arc and a shoulder — so the pair overlaps the way people in
		// a group avatar do instead of standing in a row.
		arc(p, s, 9, 7.5, 3.5)
		shoulders(p, s, 9, 2.5, 15.5)
		p.MoveTo(at(s, 15.5, 4.3))
		p.CubeTo(at(s, 17.5, 4.8), at(s, 18.5, 6), at(s, 18.5, 7.5))
		p.CubeTo(at(s, 18.5, 9), at(s, 17.5, 10.2), at(s, 15.5, 10.7))
		p.MoveTo(at(s, 17.5, 14.5))
		p.CubeTo(at(s, 20, 15.3), at(s, 21.5, 17), at(s, 21.5, 20))
	case IconPuzzle:
		// A square piece with a knob on top and one on the right. Two knobs
		// are enough to say "puzzle"; a socket as well makes the outline busy
		// at 16px.
		p.MoveTo(at(s, 4.5, 8.5))
		p.LineTo(at(s, 8, 8.5))
		p.CubeTo(at(s, 8, 6.2), at(s, 8.6, 4.5), at(s, 10, 4.5))
		p.CubeTo(at(s, 11.4, 4.5), at(s, 12, 6.2), at(s, 12, 8.5))
		p.LineTo(at(s, 15.5, 8.5))
		p.LineTo(at(s, 15.5, 12))
		p.CubeTo(at(s, 17.8, 12), at(s, 19.5, 12.6), at(s, 19.5, 14))
		p.CubeTo(at(s, 19.5, 15.4), at(s, 17.8, 16), at(s, 15.5, 16))
		p.LineTo(at(s, 15.5, 19.5))
		p.LineTo(at(s, 4.5, 19.5))
		p.Close()
	case IconGrip:
		// Dots are the round caps of near-zero segments, like IconList's
		// bullets, so they carry the same stroke weight as every other glyph.
		for _, x := range []float32{9.5, 14.5} {
			for _, y := range []float32{6, 12, 18} {
				line(p, s, x-0.3, y, x+0.3, y)
			}
		}
	case IconReply:
		p.MoveTo(at(s, 9, 5))
		p.LineTo(at(s, 4, 10))
		p.LineTo(at(s, 9, 15))
		p.MoveTo(at(s, 4, 10))
		p.LineTo(at(s, 13, 10))
		p.CubeTo(at(s, 17.5, 10), at(s, 20, 12.5), at(s, 20, 16.5))
		p.LineTo(at(s, 20, 19))
	case IconCheckCircle:
		arc(p, s, 12, 12, 9)
		p.MoveTo(at(s, 8, 12.5))
		p.LineTo(at(s, 11, 15.5))
		p.LineTo(at(s, 16.5, 9))
	case IconTrash:
		line(p, s, 4, 7, 20, 7)
		p.MoveTo(at(s, 9.5, 7))
		p.LineTo(at(s, 9.5, 4.5))
		p.LineTo(at(s, 14.5, 4.5))
		p.LineTo(at(s, 14.5, 7))
		p.MoveTo(at(s, 6, 7))
		p.LineTo(at(s, 7, 20))
		p.LineTo(at(s, 17, 20))
		p.LineTo(at(s, 18, 7))
		line(p, s, 10, 11, 10, 16.5)
		line(p, s, 14, 11, 14, 16.5)
	case IconShare:
		// The iOS shape: an arrow rising out of an open-topped tray. It is the
		// one both phone platforms' users recognise as "share" without a label.
		p.MoveTo(at(s, 8, 10))
		p.LineTo(at(s, 5, 10))
		p.LineTo(at(s, 5, 20))
		p.LineTo(at(s, 19, 20))
		p.LineTo(at(s, 19, 10))
		p.LineTo(at(s, 16, 10))
		line(p, s, 12, 14, 12, 3.5)
		p.MoveTo(at(s, 8.5, 7))
		p.LineTo(at(s, 12, 3.5))
		p.LineTo(at(s, 15.5, 7))
	case IconCopy:
		// Two sheets: the back one drawn only where the front does not cover it.
		p.MoveTo(at(s, 8, 8))
		p.LineTo(at(s, 8, 4.5))
		p.LineTo(at(s, 19.5, 4.5))
		p.LineTo(at(s, 19.5, 16))
		p.LineTo(at(s, 16, 16))
		p.MoveTo(at(s, 4.5, 8))
		p.LineTo(at(s, 16, 8))
		p.LineTo(at(s, 16, 19.5))
		p.LineTo(at(s, 4.5, 19.5))
		p.Close()
	}
}

// shoulders appends the torso arc under a head: an open curve from (x0,20) up
// over the shoulders at cx and back down to (x1,20).
func shoulders(p *paint.Path, s, cx, x0, x1 float32) {
	p.MoveTo(at(s, x0, 20))
	p.CubeTo(at(s, x0, 15.5), at(s, cx-4, 14), at(s, cx, 14))
	p.CubeTo(at(s, cx+4, 14), at(s, x1, 15.5), at(s, x1, 20))
}
