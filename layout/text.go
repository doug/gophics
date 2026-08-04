package layout

import (
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// TextBox lays out and paints text through the shaping stack (bidi,
// fallback, positional forms): single line by default, greedy word-wrap
// within the width constraint when Wrap is set, with optional
// strike/underline decorations, line limiting, and ellipsis overflow.
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
	// MaxLines caps the number of wrapped lines (0 = unlimited).
	MaxLines int
	// Ellipsis truncates overflowing text with "…": a single line to the
	// width, or a wrapped block at MaxLines.
	Ellipsis bool
	// Selection, when set (by a Text inside a widget.SelectionArea), makes this
	// text a selectable fragment: it registers each frame and paints the
	// selected runes' highlight.
	Selection SelectionSink

	lines    []string
	baseline float32 // first-line baseline
	lineH    float32 // baseline-to-baseline advance
	descent  float32
}

// ellipsize trims line until it plus "…" fits maxW; force adds the ellipsis
// even when the line already fits (used when later lines were dropped).
func (b *TextBox) ellipsize(line string, maxW float32, force bool) string {
	if !force && b.Painter.MeasureWidthIn(b.Font, line, b.TextSize) <= maxW {
		return line
	}
	runes := []rune(line)
	for len(runes) > 0 {
		trimmed := strings.TrimRight(string(runes), " ")
		if b.Painter.MeasureWidthIn(b.Font, trimmed+"…", b.TextSize) <= maxW {
			return trimmed + "…"
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
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
		if b.MaxLines > 0 && len(b.lines) > b.MaxLines {
			dropped := b.lines[:b.MaxLines]
			if b.Ellipsis && len(dropped) > 0 {
				last := len(dropped) - 1
				dropped[last] = b.ellipsize(dropped[last], cs.Max.W, true)
			}
			b.lines = dropped
		}
	} else {
		line := b.Text
		if b.Ellipsis && cs.BoundedW() {
			line = b.ellipsize(line, cs.Max.W, false)
		}
		b.lines = append(b.lines[:0], line)
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
	// Selection: register this fragment and get the rune range to highlight.
	selLo, selHi := 0, 0
	var selCol paint.Color
	if b.Selection != nil {
		selLo, selHi, selCol = b.Selection.RegisterText(TextFragment{
			Origin: at, Lines: b.lines, Font: b.Font, Size: b.TextSize,
			LineH: b.lineH, Ascent: b.baseline, Descent: b.descent, Painter: b.Painter,
		})
	}
	lineOff := 0 // linear rune offset at this line's start
	for i, ln := range b.lines {
		base := at.Y + b.baseline + float32(i)*b.lineH
		runes := []rune(ln)
		// Selection highlight for this line, painted under the glyphs.
		if selHi > selLo {
			a := max(selLo-lineOff, 0)
			z := min(selHi-lineOff, len(runes))
			if a < z {
				x0 := b.Painter.MeasureWidthIn(b.Font, string(runes[:a]), b.TextSize)
				x1 := b.Painter.MeasureWidthIn(b.Font, string(runes[:z]), b.TextSize)
				top := at.Y + float32(i)*b.lineH
				c.FillRect(geom.Rect{
					Min: geom.Pt{X: at.X + x0, Y: top},
					Max: geom.Pt{X: at.X + x1, Y: top + b.lineH},
				}, selCol)
			}
		}
		lineOff += len(runes) + 1
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
