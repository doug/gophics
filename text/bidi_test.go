package text

import (
	"strings"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/text/unicode/bidi"
)

func TestBaseDirection(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want di.Direction
	}{
		{"latin", "Hello", di.DirectionLTR},
		{"arabic", "مرحبا", di.DirectionRTL},
		{"hebrew", "שלום", di.DirectionRTL},
		// P2 takes the *first* strong character, so a leading Latin word makes
		// the whole paragraph LTR even though most of it is Arabic.
		{"latin first", "Hello مرحبا", di.DirectionLTR},
		{"arabic first", "مرحبا Hello", di.DirectionRTL},
		// Digits and punctuation are not strong: P3 falls back to LTR.
		{"neutral only", "123 — (456)", di.DirectionLTR},
		{"empty", "", di.DirectionLTR},
		// Leading neutrals are skipped rather than deciding the answer.
		{"neutral then arabic", "  \"مرحبا\"", di.DirectionRTL},
		// P2 skips characters between an isolate initiator and its PDI, so the
		// Arabic inside the isolate must not win over the Latin after it.
		{"isolated arabic then latin", "⁧مرحبا⁩ Hello", di.DirectionLTR},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := baseDirection([]rune(c.in)); got != c.want {
				t.Errorf("baseDirection(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// runSeq builds recognizable runs: the Runes.Offset carries a logical index so
// the test can assert on the order without needing real glyphs.
func runSeq(dirs ...di.Direction) []shaping.Output {
	out := make([]shaping.Output, len(dirs))
	for i, d := range dirs {
		out[i].Direction = d
		out[i].Runes.Offset = i
	}
	return out
}

func order(runs []shaping.Output) []int {
	got := make([]int, len(runs))
	for i, r := range runs {
		got[i] = r.Runes.Offset
	}
	return got
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Embedding levels as runLevel assigns them: on an LTR base RTL runs are 1 and
// LTR runs 0 (a number attached to RTL text, 2); on an RTL base RTL runs are 1
// and LTR runs 2.
func TestVisualOrder(t *testing.T) {
	cases := []struct {
		name   string
		levels []int
		want   []int
	}{
		{"ltr base, all ltr", []int{0, 0, 0}, []int{0, 1, 2}},
		// One RTL run inside LTR text: the run reverses internally (HarfBuzz
		// already did that), its position in the line does not.
		{"ltr base, one rtl", []int{0, 1, 0}, []int{0, 1, 2}},
		// Adjacent RTL runs (a script or font change mid-phrase) are one
		// sequence and swap relative order.
		{"ltr base, rtl pair", []int{0, 1, 1, 0}, []int{0, 2, 1, 3}},
		// A number inside an RTL phrase in LTR text: "abc عربي 123 نص xyz". The
		// number is level 2, so the two Arabic words and the number between
		// them reverse as one block — the number does not split the phrase
		// into two independently reversed halves.
		{"ltr base, number inside rtl", []int{0, 1, 2, 1, 0}, []int{0, 3, 2, 1, 4}},

		{"rtl base, all rtl", []int{1, 1, 1}, []int{2, 1, 0}},
		// The regression this fix is about: an English phrase inside an Arabic
		// sentence. The line runs right-to-left, so the trailing Arabic run
		// paints leftmost.
		{"rtl base, one ltr", []int{1, 2, 1}, []int{2, 1, 0}},
		// Two adjacent LTR runs keep their own left-to-right order while the
		// sentence around them reverses — this is what a single whole-line
		// reversal would get wrong.
		{"rtl base, ltr pair", []int{1, 2, 2, 1}, []int{3, 1, 2, 0}},
		{"rtl base, all ltr", []int{2, 2}, []int{0, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runs := make([]shaping.Output, len(c.levels))
			for i := range runs {
				runs[i].Runes.Offset = i
			}
			got := order(visualOrder(runs, c.levels))
			if !eq(got, c.want) {
				t.Errorf("visualOrder(levels=%v) = %v, want %v", c.levels, got, c.want)
			}
		})
	}
}

// visualOrder must not scribble on the caller's slices: Paragraph reuses the
// shaped runs across wrapped lines.
func TestVisualOrderDoesNotMutateInput(t *testing.T) {
	in := runSeq(di.DirectionRTL, di.DirectionLTR, di.DirectionRTL)
	levels := []int{1, 2, 1}
	before := order(in)
	visualOrder(in, levels)
	if !eq(order(in), before) {
		t.Errorf("input reordered: %v, was %v", order(in), before)
	}
	if !eq(levels, []int{1, 2, 1}) {
		t.Errorf("levels reordered: %v", levels)
	}
}

// The segmenter splits runs by direction, so a number after Arabic text shapes
// as one LTR run with the Latin that follows it. splitNumbers has to cut the
// number off so it can take its own (deeper) embedding level.
func TestSplitNumbersCutsNumberOffFollowingLatin(t *testing.T) {
	cases := []struct {
		in   string
		want int // rune length of the level-2 prefix of the LTR run after the Arabic
	}{
		{"عربي 123 xyz", 3},
		{"عربي 1,234.5 xyz", 7}, // W4: separators between digits join the number
		{"عربي 123, xyz", 3},    // a separator not between digits ends it (W6)
		{"עברית $12 xyz", 3},    // W5: a terminator joins a European number after Hebrew
		{"عربي $12 xyz", 0},     // but not an Arabic number (W2 came first)
		{"عربي xyz", 0},
	}
	for _, c := range cases {
		runes := []rune(c.in)
		// The LTR run starts after the RTL word and its trailing space,
		// which the bidi algorithm attaches to the RTL text.
		start := strings.IndexRune(c.in, ' ')
		start = len([]rune(c.in[:start])) + 1
		strong := strongBefore(runes, start)
		if strong == bidi.L {
			t.Fatalf("%q: strong before %d = L", c.in, start)
		}
		if got := numericPrefix(runes[start:], strong == bidi.AL); got != c.want {
			t.Errorf("numericPrefix(%q) = %d, want %d", string(runes[start:]), got, c.want)
		}
	}
}

// The three-level case end to end: "abc عربي 123 نص xyz" in an LTR paragraph
// must paint as abc · نص · 123 · عربي · xyz — the Arabic phrase reads right to
// left *around* the number, so the second Arabic word is left of the number
// and the first is right of it. goregular has no Arabic glyphs, but the
// segmenter and the reordering do not depend on the font.
func TestLineNumberInsideRTLPhrase(t *testing.T) {
	s := NewShaper(regular(t))
	str := "abc عربي 123 نص xyz"
	l := s.Line(str, 16)
	xOf := func(cluster int) float32 {
		for _, g := range l.Glyphs {
			if g.Cluster == cluster {
				return g.X
			}
		}
		t.Fatalf("no glyph for cluster %d", cluster)
		return 0
	}
	// Rune indices: abc=0..2, عربي=4..7, 123=9..11, نص=13..14, xyz=16..18.
	c, first, digit, second, x := xOf(2), xOf(4), xOf(9), xOf(13), xOf(16)
	if !(c < second && second < digit && digit < first && first < x) {
		t.Errorf("visual x: abc=%v نص=%v 123=%v عربي=%v xyz=%v; want abc < نص < 123 < عربي < xyz",
			c, second, digit, first, x)
	}
}

func TestShaperDirectionOverride(t *testing.T) {
	s := NewShaper(regular(t))
	if got := s.baseDir([]rune("مرحبا")); got != di.DirectionRTL {
		t.Errorf("auto on Arabic = %v, want RTL", got)
	}
	s.SetDirection(DirLTR)
	if got := s.baseDir([]rune("مرحبا")); got != di.DirectionLTR {
		t.Errorf("forced LTR = %v, want LTR", got)
	}
	s.SetDirection(DirRTL)
	// A forced RTL base is the case an RTL locale needs: Latin strings in the
	// UI still anchor to the right.
	if got := s.baseDir([]rune("Hello")); got != di.DirectionRTL {
		t.Errorf("forced RTL = %v, want RTL", got)
	}
}

// A paragraph's direction is resolved per paragraph, so a Hebrew line and a
// Latin line in the same block get different answers.
func TestParagraphDirectionIsPerParagraph(t *testing.T) {
	s := NewShaper(regular(t))
	lines := s.Paragraph("שלום\nHello", 14, 0)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !lines[0].RTL {
		t.Error("Hebrew line: RTL = false, want true")
	}
	if lines[1].RTL {
		t.Error("Latin line: RTL = true, want false")
	}
}
