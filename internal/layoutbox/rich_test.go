package layoutbox

import (
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// The wrapper leaves the break rune it broke at on the end of each line.
// RichBox kept it: a wrapped line's last segment was "aaaa " and its measured
// width counted the space, so the paragraph over-reported its width (past
// maxW) and a link's underline ran under the gap. TextBox trims it; RichBox
// has to agree with TextBox.
func TestRichBoxTrimsTrailingBreakRunes(t *testing.T) {
	p := textPainter(t)
	text := "aaaa bbbb cccc dddd eeee"
	rb := &RichBox{Painter: p, TextSize: 14, Spans: []layout.RichSpan{{Text: text, Link: "x"}}}
	rich := rb.Layout(layout.Loose(geom.Size{W: 60, H: 1000}))
	tb := &TextBox{Painter: p, Text: text, TextSize: 14, Wrap: true}
	plain := tb.Layout(layout.Loose(geom.Size{W: 60, H: 1000}))
	if len(rb.segs) < 2 {
		t.Fatalf("the text did not wrap: %d segments", len(rb.segs))
	}
	for _, s := range rb.segs {
		if strings.HasSuffix(s.text, " ") {
			t.Errorf("segment %q keeps its trailing space", s.text)
		}
	}
	if rich.W != plain.W {
		t.Errorf("rich width %v, TextBox width %v for the same wrapped text", rich.W, plain.W)
	}
	if rich.W > 60 {
		t.Errorf("rich width %v exceeds the wrap width 60", rich.W)
	}
	// The link's hit extent stops at the ink, not after the space.
	last := rb.segs[0]
	if _, ok := rb.LinkAt(geom.Pt{X: last.x + last.w + 2, Y: last.y - 2}); ok {
		t.Error("LinkAt hit the trailing space of a wrapped line")
	}
}

// A carriage return on a CRLF line end is a break rune too; left on the
// line it shapes as a visible glyph.
func TestRichBoxDropsCarriageReturns(t *testing.T) {
	p := textPainter(t)
	rb := &RichBox{Painter: p, TextSize: 14, Spans: []layout.RichSpan{{Text: "one\r\ntwo"}}}
	rb.Layout(layout.Loose(geom.Size{W: 1000, H: 1000}))
	for _, s := range rb.segs {
		if strings.ContainsAny(s.text, "\r\n") {
			t.Errorf("segment %q carries a line-end rune", s.text)
		}
	}
	if w := p.MeasureWidthIn("", "one", 14); rb.segs[0].w != w {
		t.Errorf("first line measured %v, want %v (the width of %q)", rb.segs[0].w, w, "one")
	}
}
