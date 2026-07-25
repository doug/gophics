package layout

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// RichSpan is one styled run of a rich paragraph.
type RichSpan struct {
	Text      string
	Font      string // named font family ("" = default)
	Color     paint.Color
	Underline bool
	// Link marks the span tappable; RichBox.LinkAt reports it.
	Link string
}

// RichBox lays out mixed-style spans as one wrapped paragraph. Wrapping
// treats the concatenated text as a unit (UAX #14 via the text package);
// styling is applied per span segment. Segments shape independently, so
// kerning across a style boundary is dropped — invisible at word-level
// boundaries, which is where styles change in practice.
type RichBox struct {
	Base
	Painter  *paint.Painter
	Spans    []RichSpan
	TextSize float32

	segs     []richSeg
	baseline float32
	lineH    float32
}

type richSeg struct {
	text string
	span int
	x, y float32 // baseline-left, box-local
	w    float32
}

func (b *RichBox) fullText() string {
	var s string
	for _, sp := range b.Spans {
		s += sp.Text
	}
	return s
}

// spanAt maps a rune index of the full text to its span index.
func (b *RichBox) spanAt(idx int) int {
	n := 0
	for i, sp := range b.Spans {
		n += len([]rune(sp.Text))
		if idx < n {
			return i
		}
	}
	return len(b.Spans) - 1
}

func (b *RichBox) Layout(cs Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	m := b.Painter.Metrics(b.TextSize)
	b.baseline, b.lineH = m.Ascent, m.LineHeight()
	full := b.fullText()
	runes := []rune(full)

	maxW := cs.Max.W
	lines := b.Painter.Paragraph(full, b.TextSize, maxW)

	b.segs = b.segs[:0]
	var width float32
	for li, line := range lines {
		y := b.baseline + float32(li)*b.lineH
		// Split the line's rune range at span boundaries; x advances by the
		// cumulative width of prior segments, each measured in its own font.
		a := line.Start
		var x float32
		for a < line.End {
			span := b.spanAt(a)
			end := line.End
			// find where this span ends within the line
			spanEnd := 0
			for i := 0; i <= span; i++ {
				spanEnd += len([]rune(b.Spans[i].Text))
			}
			if spanEnd < end {
				end = spanEnd
			}
			segText := string(runes[a:end])
			w := b.Painter.MeasureWidthIn(b.Spans[span].Font, segText, b.TextSize)
			b.segs = append(b.segs, richSeg{text: segText, span: span, x: x, y: y, w: w})
			x += w
			a = end
		}
		if x > width {
			width = x
		}
	}
	h := m.Ascent + m.Descent
	if n := len(lines); n > 1 {
		h += float32(n-1) * b.lineH
	}
	return b.Done(cs, cs.Constrain(geom.Size{W: width, H: h}))
}

func (b *RichBox) Paint(c paint.Canvas, at geom.Pt) {
	for _, s := range b.segs {
		sp := b.Spans[s.span]
		pos := geom.Pt{X: at.X + s.x, Y: at.Y + s.y}
		c.TextIn(sp.Font, s.text, pos, b.TextSize, sp.Color)
		if sp.Underline || sp.Link != "" {
			y := pos.Y + 2
			c.Line(geom.Pt{X: pos.X, Y: y}, geom.Pt{X: pos.X + s.w, Y: y}, 1, sp.Color)
		}
	}
}

// LinkAt returns the link under the box-local point, if any.
func (b *RichBox) LinkAt(p geom.Pt) (string, bool) {
	for _, s := range b.segs {
		sp := b.Spans[s.span]
		if sp.Link == "" {
			continue
		}
		r := geom.Rect{
			Min: geom.Pt{X: s.x, Y: s.y - b.baseline},
			Max: geom.Pt{X: s.x + s.w, Y: s.y - b.baseline + b.lineH},
		}
		if r.Contains(p) {
			return sp.Link, true
		}
	}
	return "", false
}

func (b *RichBox) AddHits(p geom.Pt, hits *[]Hit) {
	if b.contains(p) {
		*hits = append(*hits, Hit{b, p})
	}
}

// Semantics reports the concatenated text.
func (b *RichBox) Semantics() SemInfo {
	return SemInfo{Role: RoleText, Label: b.fullText()}
}
