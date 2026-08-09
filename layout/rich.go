package layout

import (
	"strings"
	"unicode/utf8"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
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
	spanEnds []int // cumulative rune-end index per span, rebuilt each layout
	memo     richMemo
}

// richMemo caches the last layout's shaping-relevant inputs so a re-layout
// with the same text at the same width (e.g. a height-only window resize,
// where the constraints — and thus the Base skip-cache key — change) reuses
// the computed segs instead of re-shaping the whole paragraph.
type richMemo struct {
	valid bool
	gen   uint64 // Painter.ShapeGen: font changes invalidate
	size  float32
	maxW  float32
	spans []memoSpan
	w, h  float32 // natural (pre-Constrain) size of the laid-out paragraph
}

// memoSpan is the geometry-affecting part of a RichSpan (Color/Underline/
// Link change paint only — Paint reads the live Spans — so they don't
// invalidate the memo).
type memoSpan struct{ text, font string }

type richSeg struct {
	text string
	span int
	x, y float32 // baseline-left, box-local
	w    float32
}

func (b *RichBox) fullText() string {
	var sb strings.Builder
	n := 0
	for _, sp := range b.Spans {
		n += len(sp.Text)
	}
	sb.Grow(n)
	for _, sp := range b.Spans {
		sb.WriteString(sp.Text)
	}
	return sb.String()
}

// spanAt maps a rune index of the full text to its span index, using the
// cumulative rune counts hoisted into spanEnds by Layout.
func (b *RichBox) spanAt(idx int) int {
	for i, end := range b.spanEnds {
		if idx < end {
			return i
		}
	}
	return len(b.Spans) - 1
}

// memoHit reports whether the cached segs from the last layout are still
// valid for the given wrap width.
func (b *RichBox) memoHit(maxW float32) bool {
	m := &b.memo
	if !m.valid || m.gen != b.Painter.ShapeGen() || m.size != b.TextSize ||
		m.maxW != maxW || len(m.spans) != len(b.Spans) {
		return false
	}
	for i, sp := range b.Spans {
		if m.spans[i].text != sp.Text || m.spans[i].font != sp.Font {
			return false
		}
	}
	return true
}

func (b *RichBox) memoStore(maxW, w, h float32) {
	m := &b.memo
	m.valid, m.gen, m.size, m.maxW, m.w, m.h = true, b.Painter.ShapeGen(), b.TextSize, maxW, w, h
	m.spans = m.spans[:0]
	for _, sp := range b.Spans {
		m.spans = append(m.spans, memoSpan{sp.Text, sp.Font})
	}
}

func (b *RichBox) Layout(cs Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	m := b.Painter.Metrics(b.TextSize)
	b.baseline, b.lineH = m.Ascent, m.LineHeight()

	maxW := cs.Max.W
	if b.memoHit(maxW) {
		// Same text, fonts, size, and wrap width as last layout (only the
		// height constraint changed, say): b.segs are still valid.
		return b.Done(cs, cs.Constrain(geom.Size{W: b.memo.w, H: b.memo.h}))
	}
	full := b.fullText()
	runes := []rune(full)

	// Hoist per-span rune counts once: spanAt and the segment splitter below
	// previously re-converted []rune(sp.Text) on every lookup.
	b.spanEnds = b.spanEnds[:0]
	n := 0
	for _, sp := range b.Spans {
		n += utf8.RuneCountInString(sp.Text)
		b.spanEnds = append(b.spanEnds, n)
	}

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
			// clip the segment to where this span ends
			if spanEnd := b.spanEnds[span]; spanEnd < end {
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
	b.memoStore(maxW, width, h)
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
