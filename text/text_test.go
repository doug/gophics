package text

import (
	"os"
	"runtime"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func regular(t *testing.T) *Font {
	t.Helper()
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func arabicFont(t *testing.T) *Font {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("Arabic tests use a macOS system font")
	}
	data, err := os.ReadFile("/System/Library/Fonts/SFArabic.ttf")
	if err != nil {
		t.Skip("SFArabic.ttf not present")
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLatinLine(t *testing.T) {
	s := NewShaper(regular(t))
	l := s.Line("Hello, world", 14)
	if len(l.Glyphs) == 0 || l.Width <= 0 {
		t.Fatalf("line = %+v", l)
	}
	if l.Ascent <= 0 || l.Descent <= 0 {
		t.Fatalf("bounds ascent=%v descent=%v", l.Ascent, l.Descent)
	}
	// Positions are monotonically nondecreasing for pure LTR.
	for i := 1; i < len(l.Glyphs); i++ {
		if l.Glyphs[i].X < l.Glyphs[i-1].X {
			t.Fatalf("LTR glyph positions must not go backward: %v then %v",
				l.Glyphs[i-1].X, l.Glyphs[i].X)
		}
	}
	if l.Start != 0 || l.End != len([]rune("Hello, world")) {
		t.Fatalf("rune range = [%d,%d)", l.Start, l.End)
	}
}

func TestArabicShapingSubstitutes(t *testing.T) {
	ar := arabicFont(t)
	s := NewShaper(ar)
	const word = "السلام"
	l := s.Line(word, 28)
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	nominal := map[uint32]bool{}
	for _, r := range word {
		if gid, ok := ar.NominalGID(r); ok {
			nominal[gid] = true
		}
	}
	substituted := 0
	for _, g := range l.Glyphs {
		if !nominal[g.GID] {
			substituted++
		}
	}
	if substituted == 0 {
		t.Fatal("expected positional forms: no glyph differs from nominal cmap glyphs")
	}
	t.Logf("glyphs=%d substituted=%d (gg's shaper produced 0)", len(l.Glyphs), substituted)
}

func TestBidiWithFallback(t *testing.T) {
	ar := arabicFont(t)
	s := NewShaper(regular(t), ar)
	l := s.Line("abc السلام xyz", 16)

	// The Arabic segment must come from the fallback font.
	arabicGlyphs := 0
	for _, g := range l.Glyphs {
		if g.Font == ar {
			arabicGlyphs++
		}
	}
	if arabicGlyphs == 0 {
		t.Fatal("fallback font not used for Arabic segment")
	}

	// Visual order: RTL segment glyphs are positioned between the Latin
	// segments, with clusters *decreasing* along x within the Arabic run.
	var arXs []float32
	var arClusters []int
	for _, g := range l.Glyphs {
		if g.Font == ar {
			arXs = append(arXs, g.X)
			arClusters = append(arClusters, g.Cluster)
		}
	}
	for i := 1; i < len(arXs); i++ {
		if arXs[i] < arXs[i-1] {
			t.Fatal("run glyphs should advance left-to-right in device space")
		}
		if arClusters[i] > arClusters[i-1] {
			t.Fatalf("RTL clusters must decrease along x: %v", arClusters)
		}
	}
}

func TestParagraphWrapping(t *testing.T) {
	s := NewShaper(regular(t))
	str := "the quick brown fox jumps over the lazy dog"
	one := s.Paragraph(str, 14, 10000)
	if len(one) != 1 {
		t.Fatalf("wide wrap lines = %d", len(one))
	}
	many := s.Paragraph(str, 14, 100)
	if len(many) < 3 {
		t.Fatalf("narrow wrap lines = %d, want several", len(many))
	}
	for _, l := range many {
		if l.Width > 100+1 {
			t.Fatalf("line width %v exceeds wrap width", l.Width)
		}
	}
	// Rune ranges tile the string (with breaks at spaces).
	if many[0].Start != 0 {
		t.Fatal("first line must start at 0")
	}
	for i := 1; i < len(many); i++ {
		if many[i].Start < many[i-1].End {
			t.Fatalf("line ranges overlap: %d < %d", many[i].Start, many[i-1].End)
		}
	}
}

func TestParagraphNewlines(t *testing.T) {
	s := NewShaper(regular(t))
	lines := s.Paragraph("a\nb\n\nc", 14, 0)
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4 (including empty)", len(lines))
	}
	if lines[2].Width != 0 {
		t.Fatal("empty line must have zero width")
	}
}

func TestGlyphPathEmits(t *testing.T) {
	f := regular(t)
	gid, ok := f.NominalGID('o')
	if !ok {
		t.Fatal("no glyph for 'o'")
	}
	var sink countSink
	f.AppendGlyphPath(&sink, gid, 24, 0, 0)
	// 'o' has two contours (outer ring + counter).
	if sink.moves < 2 || sink.closes < 2 || sink.curves == 0 {
		t.Fatalf("path ops = %+v, want ≥2 contours with curves", sink)
	}
}

type countSink struct {
	moves, lines, curves, closes int
}

func (c *countSink) MoveTo(x, y float32)             { c.moves++ }
func (c *countSink) LineTo(x, y float32)             { c.lines++ }
func (c *countSink) QuadTo(cx, cy, x, y float32)     { c.curves++ }
func (c *countSink) CubeTo(a, b, d, e, x, y float32) { c.curves++ }
func (c *countSink) Close()                          { c.closes++ }

// TestSystemFontFallback covers two claims that fail for unrelated reasons, so
// it makes them separately.
//
// Routing — CJK leaves the primary font and goes to the system map — is ours,
// and holds on any machine: with no CJK font installed fontscan still answers
// with some arbitrary face, which is enough to show the handoff happened.
//
// Coverage — the glyph that comes back is real rather than .notdef — is the
// machine's. A bare CI runner has no CJK font, and fontscan says as much
// ("No font matched for script … returning arbitrary face") before handing
// back a face whose glyph for 你 is 0. That is a missing font, not a bug, and
// it failed this test for weeks on ubuntu-latest.
//
// So coverage skips by default and is required where the fonts are known to be
// installed, via GOPHICS_REQUIRE_SYSTEM_CJK=1 — which CI sets, having just
// apt-installed them. Otherwise a genuine coverage regression would skip
// silently on the one machine positioned to catch it.
func TestSystemFontFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("system font scan")
	}
	s := NewShaper(regular(t))
	if err := s.UseSystemFonts(t.TempDir()); err != nil {
		t.Skipf("system fonts unavailable: %v", err)
	}
	// CJK is not in Go Regular; it must resolve through the system map.
	l := s.Line("你好 hello", 16)
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	var system []Glyph
	for _, g := range l.Glyphs {
		if g.Font != s.Primary() && g.Font != nil {
			system = append(system, g)
		}
	}
	if len(system) == 0 {
		t.Fatal("CJK runes did not use a system font")
	}

	for _, g := range system {
		if g.GID != 0 {
			continue
		}
		if os.Getenv("GOPHICS_REQUIRE_SYSTEM_CJK") == "1" {
			t.Fatalf("system glyph is .notdef, and GOPHICS_REQUIRE_SYSTEM_CJK=1 "+
				"says a CJK font should be installed (glyph %+v)", g)
		}
		t.Skip("no installed system font covers U+4F60 — on Debian/Ubuntu, " +
			"apt-get install fonts-noto-cjk")
	}
}
