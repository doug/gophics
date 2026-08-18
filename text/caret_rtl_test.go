package text

import "testing"

// In a right-to-left run the caret before a cluster sits at that glyph's right
// edge: "before" means earlier in reading order, and reading runs the other
// way. An LTR-only implementation puts it on the far side of the character
// being typed, which is the classic symptom.
func TestCaretXRTL(t *testing.T) {
	// 5 clusters, 10px each. Cluster 0 is the rightmost glyph, at X=40.
	l := rtlLine10(5)
	for idx, want := range map[int]float32{
		0: 50, // before the first character: the right edge of the line
		1: 40,
		2: 30,
		4: 10,
	} {
		if got := l.CaretX(idx); got != want {
			t.Errorf("CaretX(%d) on an RTL line = %v, want %v", idx, got, want)
		}
	}
}

// Past the last cluster the caret belongs at the line's trailing edge, which is
// the left on an RTL line and the right on an LTR one.
func TestCaretXTrailingEdge(t *testing.T) {
	if got := rtlLine10(5).CaretX(5); got != 0 {
		t.Errorf("RTL end caret = %v, want 0 (the left edge)", got)
	}
	if got := line10(5).CaretX(5); got != 50 {
		t.Errorf("LTR end caret = %v, want 50 (the right edge)", got)
	}
}

// A bidi line mixes runs, and each glyph's direction has to be read from its
// visual neighbours rather than from the line's base direction.
func TestCaretXMixedRuns(t *testing.T) {
	// "ab" LTR then an RTL run of clusters 4,3,2 laid out left to right.
	l := Line{
		Glyphs: []Glyph{
			{Cluster: 0, X: 0, Advance: 10},
			{Cluster: 1, X: 10, Advance: 10},
			{Cluster: 4, X: 20, Advance: 10},
			{Cluster: 3, X: 30, Advance: 10},
			{Cluster: 2, X: 40, Advance: 10},
		},
		Width: 50, End: 5,
	}
	// LTR glyphs: caret on the left edge.
	if got := l.CaretX(0); got != 0 {
		t.Errorf("CaretX(0) = %v, want 0", got)
	}
	if got := l.CaretX(1); got != 10 {
		t.Errorf("CaretX(1) = %v, want 10", got)
	}
	// RTL glyphs: caret on the right edge of the glyph.
	if got := l.CaretX(2); got != 50 {
		t.Errorf("CaretX(2) = %v, want 50 (right edge of the RTL glyph at 40)", got)
	}
	if got := l.CaretX(4); got != 30 {
		t.Errorf("CaretX(4) = %v, want 30 (right edge of the RTL glyph at 20)", got)
	}
}

// CaretX reads glyph positions, so a shaped line must actually carry them. If
// shaping ever stopped populating X, every caret would collapse to 0 — this is
// the assumption the implementation rests on.
func TestShapedGlyphsCarryPositions(t *testing.T) {
	l := NewShaper(regular(t)).Line("hello", 16)
	if len(l.Glyphs) < 2 {
		t.Fatal("expected several glyphs")
	}
	if l.Glyphs[1].X == 0 {
		t.Error("shaped glyphs have no X positions; CaretX cannot use them")
	}
}
