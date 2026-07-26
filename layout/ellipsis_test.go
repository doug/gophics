package layout

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

func tbPainter(t *testing.T) *paint.Painter {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSingleLineEllipsis(t *testing.T) {
	p := tbPainter(t)
	long := "this is a very long single line of text that will not fit"
	tb := &TextBox{Painter: p, Text: long, TextSize: 14, Ellipsis: true}
	tb.Layout(Loose(geom.Size{W: 120, H: 100}))
	if len(tb.lines) != 1 {
		t.Fatalf("lines = %d", len(tb.lines))
	}
	if !strings.HasSuffix(tb.lines[0], "…") {
		t.Fatalf("expected ellipsis, got %q", tb.lines[0])
	}
	if w := p.MeasureWidth(tb.lines[0], 14); w > 120 {
		t.Fatalf("ellipsized width %v exceeds 120", w)
	}
	// Fits: no ellipsis.
	tb2 := &TextBox{Painter: p, Text: "short", TextSize: 14, Ellipsis: true}
	tb2.MarkLayoutDirty()
	tb2.Layout(Loose(geom.Size{W: 120, H: 100}))
	if strings.HasSuffix(tb2.lines[0], "…") {
		t.Fatalf("short text should not ellipsize: %q", tb2.lines[0])
	}
}

func TestMaxLinesWithEllipsis(t *testing.T) {
	p := tbPainter(t)
	para := "the quick brown fox jumps over the lazy dog and keeps running well past the edge of the box"
	tb := &TextBox{Painter: p, Text: para, TextSize: 14, Wrap: true, MaxLines: 2, Ellipsis: true}
	tb.Layout(Loose(geom.Size{W: 120, H: 1000}))
	if len(tb.lines) != 2 {
		t.Fatalf("lines = %d, want 2 (capped)", len(tb.lines))
	}
	if !strings.HasSuffix(tb.lines[1], "…") {
		t.Fatalf("last kept line should end with ellipsis: %q", tb.lines[1])
	}
}
