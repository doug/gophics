package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// selectRange drags to select the character range and copies it, returning
// the clipboard text. x positions come from the real painter so the drag
// lands on exact glyph boundaries.
func selectRange(h *Headless, text string, from, to int, y float32) string {
	p := h.core.Painter
	x0 := p.MeasureWidth(text[:from], 14)
	x1 := p.MeasureWidth(text[:to], 14)
	h.DragTo(geom.Pt{X: x0, Y: y}, geom.Pt{X: x1, Y: y})
	h.Release(geom.Pt{X: x1, Y: y})
	h.KeyMod(shell.KeyC, shell.ModSuper)
	return clip(h)
}

func TestSelectablePartialRanges(t *testing.T) {
	const s = "Hello World"
	// Ranges wider than the 4px tap slop (a sub-slop micro-drag registers as
	// a click, not a selection — the same as any drag-select UI).
	cases := []struct{ from, to int }{
		{6, 11}, // "World"
		{0, 5},  // "Hello"
		{3, 8},  // "lo Wo"
		{6, 7},  // "W" (a single wide glyph)
	}
	for _, c := range cases {
		h := selHarness(t, s)
		got := selectRange(h, s, c.from, c.to, 8)
		want := s[c.from:c.to]
		if got != want {
			t.Fatalf("select [%d,%d]: got %q, want %q", c.from, c.to, got, want)
		}
	}
}

// doubleTapAt taps twice at the same point with no time between, so the
// gesture host recognizes a double-tap.
func doubleTapAt(h *Headless, x, y float32) {
	h.Tap(geom.Pt{X: x, Y: y})
	h.Tap(geom.Pt{X: x, Y: y})
}

func TestSelectableDoubleTapSelectsWord(t *testing.T) {
	const s = "Hello World"

	// Double-tap inside "World" → selects the whole word.
	h := selHarness(t, s)
	mid := h.core.Painter.MeasureWidth("Hello Wor", 14) // inside "World"
	doubleTapAt(h, mid, 8)
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "World" {
		t.Fatalf("double-tap in World copied %q, want \"World\"", got)
	}

	// Double-tap inside "Hello" → selects "Hello".
	h2 := selHarness(t, s)
	doubleTapAt(h2, h2.core.Painter.MeasureWidth("He", 14), 8)
	h2.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h2); got != "Hello" {
		t.Fatalf("double-tap in Hello copied %q, want \"Hello\"", got)
	}
}

func TestSelectableDoubleTapAtBoundarySelectsNearestWord(t *testing.T) {
	const s = "Hello World"
	h := selHarness(t, s)
	// A tap at the single-space boundary rounds to an adjacent word offset,
	// so double-tap grabs the nearest whole word (never crashes or empties).
	x := (h.core.Painter.MeasureWidth("Hello", 14) + h.core.Painter.MeasureWidth("Hello ", 14)) / 2
	doubleTapAt(h, x, 8)
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "Hello" && got != "World" {
		t.Fatalf("double-tap at the word boundary copied %q, want an adjacent word", got)
	}
}

func TestSelectableDoubleTapInGapSelectsNothing(t *testing.T) {
	const s = "Hi   there" // three spaces — a genuine whitespace run
	h := selHarness(t, s)
	// Middle of the gap: the offset lands on a space flanked by spaces.
	x := (h.core.Painter.MeasureWidth("Hi ", 14) + h.core.Painter.MeasureWidth("Hi  ", 14)) / 2
	doubleTapAt(h, x, 8)
	h.core.Owner.Clipboard.(*MemClipboard).S = "SENTINEL"
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "SENTINEL" {
		t.Fatalf("double-tap inside a whitespace run selected %q, want nothing", got)
	}
}

func TestSelectableClickCollapsesSelection(t *testing.T) {
	const s = "Hello World"
	h := selHarness(t, s)

	// Select something, copy — works.
	if got := selectRange(h, s, 0, 5, 8); got != "Hello" {
		t.Fatalf("initial select = %q", got)
	}
	// A plain click (no drag) collapses the selection; a later copy is empty.
	h.Tap(geom.Pt{X: 20, Y: 8})
	// overwrite clipboard sentinel to detect that copy writes nothing
	h.core.Owner.Clipboard.(*MemClipboard).S = "SENTINEL"
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "SENTINEL" {
		t.Fatalf("copy with collapsed selection changed clipboard to %q", got)
	}
}

// Wrapped, multi-line selection: copy should join lines with newlines.
type wrapSelApp struct{ text string }

func (a wrapSelApp) CreateState() widget.State { return &wrapSelState{text: a.text} }

type wrapSelState struct {
	widget.StateBase[wrapSelApp]
	text string
}

func (s *wrapSelState) Build(widget.Ctx) widget.Widget {
	// The narrow surface constrains width, so Wrap breaks it into lines.
	return widget.SelectableText{S: s.text, Size: 14, Wrap: true}
}

func TestSelectableMultiLineCopyJoinsWithNewlines(t *testing.T) {
	// Width forces wrapping of this into several lines.
	h, err := NewHeadless(wrapSelApp{text: "alpha beta gamma delta epsilon zeta"},
		Config{Size: geom.Size{W: 120, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Select everything across the wrapped lines.
	h.DragTo(geom.Pt{X: 0, Y: 4}, geom.Pt{X: 3000, Y: 190})
	h.Release(geom.Pt{X: 3000, Y: 190})
	h.KeyMod(shell.KeyC, shell.ModSuper)
	got := clip(h)

	if !containsNewline(got) {
		t.Fatalf("multi-line selection should contain a newline, got %q", got)
	}
	// Joining the lines back must reproduce all the words in order.
	flat := replaceNewlines(got)
	if flat != "alpha beta gamma delta epsilon zeta" {
		t.Fatalf("multi-line copy flattened = %q", flat)
	}
}

func containsNewline(s string) bool {
	for _, r := range s {
		if r == '\n' {
			return true
		}
	}
	return false
}

func replaceNewlines(s string) string {
	out := []rune{}
	for _, r := range s {
		if r == '\n' {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}
