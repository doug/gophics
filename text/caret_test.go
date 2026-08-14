package text

import "testing"

// line10 builds a synthetic line of n clusters, each 10px wide, so caret maths
// can be checked against exact expected values rather than font metrics.
func line10(n int) Line {
	gs := make([]Glyph, n)
	for i := range gs {
		gs[i] = Glyph{Cluster: i, Advance: 10}
	}
	return Line{Glyphs: gs, Start: 0, End: n, Width: float32(n) * 10}
}

// TestCaretXIsExact pins the caret's position to the sum of the advances before
// it — an off-by-one here shifts the caret a whole character, which is the most
// visible bug a text field can have.
func TestCaretXIsExact(t *testing.T) {
	l := line10(5)
	for i := 0; i <= 5; i++ {
		if got, want := l.CaretX(i), float32(i*10); got != want {
			t.Errorf("CaretX(%d) = %v, want %v", i, got, want)
		}
	}
}

// TestIndexAtInvertsCaretX checks click-to-caret and caret-drawing agree: a click
// in the left half of a cluster lands before it, the right half after it. If these
// disagree the caret appears to jump a character when you click.
func TestIndexAtInvertsCaretX(t *testing.T) {
	l := line10(5)
	cases := []struct {
		x    float32
		want int
	}{
		{-5, 0}, {0, 0}, {4, 0}, // before the first cluster's midpoint
		{6, 1}, {14, 1}, // past it, before the second's
		{16, 2},
		{44, 4}, // last cluster's left half
		{46, 5}, // its right half: caret goes after
		{100, 5},
	}
	for _, c := range cases {
		if got := l.IndexAt(c.x); got != c.want {
			t.Errorf("IndexAt(%v) = %d, want %d", c.x, got, c.want)
		}
	}

	// Exactly on a midpoint the caret goes to the later position (round half
	// right), which is the convention click-to-caret has to pick one way or the
	// other; pinned so it cannot drift.
	if got := l.IndexAt(45); got != 5 {
		t.Errorf("IndexAt at an exact midpoint = %d, want the later index 5", got)
	}

	// Round-tripping every caret position must be stable: the x for an index,
	// nudged into that cluster, resolves back to the same index.
	for i := 0; i < 5; i++ {
		x := l.CaretX(i) + 1 // just inside cluster i
		if got := l.IndexAt(x); got != i {
			t.Errorf("IndexAt(CaretX(%d)+1) = %d, want %d", i, got, i)
		}
	}
}

// TestCaretXWithLineOffset covers wrapped lines, where indices are relative to
// the line but glyph clusters are absolute over the whole string.
func TestCaretXWithLineOffset(t *testing.T) {
	gs := make([]Glyph, 3)
	for i := range gs {
		gs[i] = Glyph{Cluster: 10 + i, Advance: 10} // this line starts at rune 10
	}
	l := Line{Glyphs: gs, Start: 10, End: 13}

	for i := 0; i <= 3; i++ {
		if got, want := l.CaretX(i), float32(i*10); got != want {
			t.Errorf("CaretX(%d) on an offset line = %v, want %v", i, got, want)
		}
	}
}
