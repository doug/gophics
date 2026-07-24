package layout

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// TextBox lays out and paints a single line of text. (Wrapping paragraphs
// arrive with the text package's paragraph layouter; see PLAN.md §6.1.)
//
// Painter supplies measurement and must be the painter used at paint time;
// the widget layer injects it from the app context.
type TextBox struct {
	Base
	Painter  *paint.Painter
	Text     string
	TextSize float32
	Color    paint.Color

	baseline float32
}

func (b *TextBox) Layout(cs Constraints) geom.Size {
	m := b.Painter.Metrics(b.TextSize)
	w := b.Painter.MeasureWidth(b.Text, b.TextSize)
	b.baseline = m.Ascent
	return b.setSize(cs.Constrain(geom.Size{W: w, H: m.Ascent + m.Descent}))
}

func (b *TextBox) Paint(c *paint.Canvas, at geom.Pt) {
	c.Text(b.Text, geom.Pt{X: at.X, Y: at.Y + b.baseline}, b.TextSize, b.Color)
}

func (b *TextBox) AddHits(p geom.Pt, hits *[]Hit) {
	if !b.contains(p) {
		return
	}
	*hits = append(*hits, Hit{b, p})
}
