package text

import (
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
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

func TestVisualOrder(t *testing.T) {
	const L, R = di.DirectionLTR, di.DirectionRTL
	cases := []struct {
		name string
		base di.Direction
		runs []di.Direction
		want []int
	}{
		{"ltr base, all ltr", L, []di.Direction{L, L, L}, []int{0, 1, 2}},
		// One RTL run inside LTR text: the run reverses internally (HarfBuzz
		// already did that), its position in the line does not.
		{"ltr base, one rtl", L, []di.Direction{L, R, L}, []int{0, 1, 2}},
		// Adjacent RTL runs (a script or font change mid-phrase) are one
		// sequence and swap relative order.
		{"ltr base, rtl pair", L, []di.Direction{L, R, R, L}, []int{0, 2, 1, 3}},

		{"rtl base, all rtl", R, []di.Direction{R, R, R}, []int{2, 1, 0}},
		// The regression this fix is about: an English phrase inside an Arabic
		// sentence. The line runs right-to-left, so the trailing Arabic run
		// paints leftmost.
		{"rtl base, one ltr", R, []di.Direction{R, L, R}, []int{2, 1, 0}},
		// Two adjacent LTR runs keep their own left-to-right order while the
		// sentence around them reverses — this is what a single whole-line
		// reversal would get wrong.
		{"rtl base, ltr pair", R, []di.Direction{R, L, L, R}, []int{3, 1, 2, 0}},
		{"rtl base, all ltr", R, []di.Direction{L, L}, []int{0, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := order(visualOrder(runSeq(c.runs...), c.base))
			if !eq(got, c.want) {
				t.Errorf("visualOrder(%v, base=%v) = %v, want %v", c.runs, c.base, got, c.want)
			}
		})
	}
}

// visualOrder must not scribble on the caller's slice: Paragraph reuses the
// shaped runs across wrapped lines.
func TestVisualOrderDoesNotMutateInput(t *testing.T) {
	in := runSeq(di.DirectionRTL, di.DirectionLTR, di.DirectionRTL)
	before := order(in)
	visualOrder(in, di.DirectionRTL)
	if !eq(order(in), before) {
		t.Errorf("input reordered: %v, was %v", order(in), before)
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
