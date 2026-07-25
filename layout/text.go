package layout

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// TextBox lays out and paints text: single-line by default, greedy
// word-wrap within the width constraint when Wrap is set, with optional
// strike/underline decorations. (Shaped paragraphs — bidi, grapheme
// breaking — arrive with the text package; PLAN.md §6.1.)
//
// Painter supplies measurement and must be the painter used at paint time;
// the widget layer injects it from the app context.
type TextBox struct {
	Base
	Painter   *paint.Painter
	Text      string
	Font      string // named font family ("" = default)
	TextSize  float32
	Color     paint.Color
	Wrap      bool
	Strike    bool
	Underline bool

	lines    []string
	baseline float32 // first-line baseline
	lineH    float32 // baseline-to-baseline advance
	descent  float32
}

func (b *TextBox) Layout(cs Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	m := b.Painter.MetricsIn(b.Font, b.TextSize)
	b.baseline = m.Ascent
	b.lineH = m.LineHeight()
	b.descent = m.Descent

	if b.Wrap && cs.BoundedW() {
		b.lines = b.Painter.WrapTextIn(b.Font, b.Text, b.TextSize, cs.Max.W)
	} else {
		b.lines = append(b.lines[:0], b.Text)
	}
	var w float32
	for _, ln := range b.lines {
		if lw := b.Painter.MeasureWidthIn(b.Font, ln, b.TextSize); lw > w {
			w = lw
		}
	}
	h := m.Ascent + m.Descent + float32(len(b.lines)-1)*b.lineH
	return b.Done(cs, cs.Constrain(geom.Size{W: w, H: h}))
}

func (b *TextBox) Paint(c paint.Canvas, at geom.Pt) {
	for i, ln := range b.lines {
		base := at.Y + b.baseline + float32(i)*b.lineH
		c.TextIn(b.Font, ln, geom.Pt{X: at.X, Y: base}, b.TextSize, b.Color)
		if !b.Strike && !b.Underline {
			continue
		}
		w := b.Painter.MeasureWidthIn(b.Font, ln, b.TextSize)
		if b.Strike {
			y := base - b.baseline*0.3
			c.Line(geom.Pt{X: at.X, Y: y}, geom.Pt{X: at.X + w, Y: y}, 1, b.Color)
		}
		if b.Underline {
			y := base + b.descent*0.6
			c.Line(geom.Pt{X: at.X, Y: y}, geom.Pt{X: at.X + w, Y: y}, 1, b.Color)
		}
	}
}

func (b *TextBox) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	*hits = append(*hits, Hit{b, p})
}
