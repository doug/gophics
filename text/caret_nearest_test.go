package text

import (
	"math"
	"os"
	"runtime"
	"testing"
)

// mixedLine is "ab" LTR, an RTL run of clusters 4,3,2, then "ef" LTR — the
// shape UAX #9 rule L2 produces for Latin around a Hebrew word — with every
// cluster 10 wide.
func mixedLine() Line {
	return Line{
		Glyphs: []Glyph{
			{Cluster: 0, X: 0, Advance: 10},
			{Cluster: 1, X: 10, Advance: 10},
			{Cluster: 4, X: 20, Advance: 10, RTL: true},
			{Cluster: 3, X: 30, Advance: 10, RTL: true},
			{Cluster: 2, X: 40, Advance: 10, RTL: true},
			{Cluster: 5, X: 50, Advance: 10},
			{Cluster: 6, X: 60, Advance: 10},
		},
		Width: 70, End: 7,
	}
}

// nearestCaret is the index whose drawn caret is closest to x, ties to the
// later index — the definition IndexAt has to meet.
func nearestCaret(l Line, x float32) int {
	best, bestD := 0, math.Inf(1)
	for i := 0; i <= l.End-l.Start; i++ {
		if d := math.Abs(float64(l.CaretX(i) - x)); d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}

// checkIndexAtIsNearest sweeps x across the line and reports every click
// whose caret lands farther away than the nearest one.
func checkIndexAtIsNearest(t *testing.T, name string, l Line) {
	t.Helper()
	bad := 0
	for x := float32(-2); x <= l.Width+2; x += 0.5 {
		got := l.IndexAt(x)
		want := nearestCaret(l, x)
		if gd, wd := math.Abs(float64(l.CaretX(got)-x)), math.Abs(float64(l.CaretX(want)-x)); gd > wd+0.01 {
			if bad < 4 {
				t.Errorf("%s: IndexAt(%.1f) = %d, drawn at %.1f; the nearest caret is %d at %.1f", name, x, got, l.CaretX(got), want, l.CaretX(want))
			}
			bad++
		}
	}
	if bad > 4 {
		t.Errorf("%s: %d click positions in all land away from the nearest caret", name, bad)
	}
}

// At a bidi run boundary the index "after" a glyph in reading order can be
// drawn at the far end of the line, so deciding a click per glyph half put
// the caret a whole run away from it. IndexAt is CaretX's inverse: the
// caret it returns is the nearest one to the click, wherever that is.
func TestIndexAtPicksTheNearestCaretAcrossRuns(t *testing.T) {
	checkIndexAtIsNearest(t, "mixed", mixedLine())
	checkIndexAtIsNearest(t, "ltr", line10(5))
	checkIndexAtIsNearest(t, "rtl", rtlLine10(5))

	// The audit's worst case: an RTL-base line of Latin and digits, as an
	// RTL-locale UI shows a numeric value. Clicking the right half of the
	// last glyph returned 7, drawn at x=0.
	s := NewShaper(regular(t))
	s.SetDirection(DirRTL)
	l := s.Line("abc 123", 16)
	checkIndexAtIsNearest(t, "abc 123 (RTL base)", l)
	if got := l.IndexAt(l.Width - 1); math.Abs(float64(l.CaretX(got)-(l.Width-1))) > 8 {
		t.Errorf("click at the right edge of %q → %d, drawn at %.1f (line width %.1f)", "abc 123", got, l.CaretX(got), l.Width)
	}
}

// A ligature fuses several runes into one glyph, and an index inside it —
// where Editor.Move lands, since f and i are separate graphemes — matched no
// glyph and fell through to the end of the line. It is interpolated across
// the cluster instead, and IndexAt finds it.
func TestCaretXInsideACluster(t *testing.T) {
	// "fi" as one glyph 20 wide covering runes 0 and 1, then "x".
	l := Line{
		Glyphs: []Glyph{
			{Cluster: 0, X: 0, Advance: 20},
			{Cluster: 2, X: 20, Advance: 10},
		},
		Width: 30, End: 3,
	}
	for idx, want := range map[int]float32{0: 0, 1: 10, 2: 20, 3: 30} {
		if got := l.CaretX(idx); got != want {
			t.Errorf("CaretX(%d) = %v, want %v", idx, got, want)
		}
	}
	for x, want := range map[float32]int{3: 0, 8: 1, 12: 1, 17: 2, 28: 3} {
		if got := l.IndexAt(x); got != want {
			t.Errorf("IndexAt(%v) = %d, want %d", x, got, want)
		}
	}

	// The same inside an RTL cluster: the caret walks from the right edge.
	r := Line{
		Glyphs: []Glyph{{Cluster: 0, X: 0, Advance: 20, RTL: true}},
		Width:  20, End: 2, RTL: true,
	}
	for idx, want := range map[int]float32{0: 20, 1: 10, 2: 0} {
		if got := r.CaretX(idx); got != want {
			t.Errorf("RTL CaretX(%d) = %v, want %v", idx, got, want)
		}
	}
}

// ligatureFont finds an installed font that shapes "fi" as one glyph. Go's
// bundled fonts have no ligatures, so this needs the system's.
func ligatureFont(t *testing.T) *Font {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("ligature tests use a macOS system font")
	}
	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/Apple Chancery.ttf",
		"/System/Library/Fonts/Supplemental/Hoefler Text.ttc",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := Parse(data)
		if err != nil {
			continue
		}
		if l := NewShaper(f).Line("fi", 20); len(l.Glyphs) == 1 {
			return f
		}
	}
	t.Skip("no installed font with an fi ligature (capability, not a defect)")
	return nil
}

func TestCaretXInsideAShapedLigature(t *testing.T) {
	s := NewShaper(ligatureFont(t))
	l := s.Line("fi and more", 20)
	x1 := l.CaretX(1)
	if x1 <= 0 || x1 >= l.CaretX(2) {
		t.Errorf("caret between f and i drawn at %.2f; want strictly between 0 and %.2f (after fi)", x1, l.CaretX(2))
	}
	if got := l.IndexAt(x1); got != 1 {
		t.Errorf("IndexAt(CaretX(1)) = %d, want 1", got)
	}
	var e Editor
	e.SetText("fi and more")
	e.MoveTo(2, false)
	e.Move(-1, false)
	if e.Caret() != 1 || l.CaretX(e.Caret()) >= l.Width/2 {
		t.Errorf("Left from after 'fi' put the caret at %d, drawn at %.2f of %.2f", e.Caret(), l.CaretX(e.Caret()), l.Width)
	}
}

// A run of one RTL glyph inside an LTR line has monotone neighbours, so its
// direction cannot be inferred from them; it comes from shaping. With it
// inferred, the caret before a lone Arabic letter sat at its left edge —
// the "after" side for a letter read right to left — and typing before it
// appeared on the other side of the caret.
func TestLoneRTLGlyphCaretEdges(t *testing.T) {
	l := Line{
		Glyphs: []Glyph{
			{Cluster: 0, X: 0, Advance: 10},
			{Cluster: 1, X: 10, Advance: 10, RTL: true},
			{Cluster: 2, X: 20, Advance: 10},
		},
		Width: 30, End: 3,
	}
	// Before the letter is its right edge. After it is, in reading order,
	// before the next LTR cluster — the same boundary, there being no caret
	// affinity — and never to the right of "before".
	if before, after := l.CaretX(1), l.CaretX(2); before != 20 || after > before {
		t.Errorf("lone RTL glyph at 10..20: caret before = %v, after = %v; want 20 and at most 20", before, after)
	}
	if _, err := os.Stat("/System/Library/Fonts/SFArabic.ttf"); err != nil {
		return // the shaped half needs the system Arabic font
	}
	s := NewShaper(regular(t), arabicFont(t))
	sl := s.Line("ab م cd", 20)
	if sl.CaretX(3) < sl.CaretX(4) {
		t.Errorf("shaped: caret before the Arabic letter at %.1f, after at %.1f; before should be the right edge", sl.CaretX(3), sl.CaretX(4))
	}
	checkIndexAtIsNearest(t, "ab م cd", sl)
}
