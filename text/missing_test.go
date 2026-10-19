package text

import (
	"bytes"
	"log"
	"testing"
)

// Go Regular covers Latin, Greek and Cyrillic and nothing else, so these runes
// have no glyph anywhere in a chain that holds only it. If that ever changes
// the first assertion here fails loudly rather than the test passing vacuously.
func TestMissingGlyphReportsOncePerScript(t *testing.T) {
	f := regular(t)
	for _, r := range []rune{'中', 'あ', '😀'} {
		if f.HasGlyph(r) {
			t.Fatalf("Go Regular grew a glyph for U+%04X; pick another rune", r)
		}
	}

	s := NewShaper(f)
	var got []Script
	s.OnMissingGlyph(func(_ rune, sc Script) { got = append(got, sc) })

	// Each script appears many times and in several shaping passes; the
	// report is per script, not per rune and not per call.
	for range 3 {
		s.Line("中中中文 ああ 😀😀", 16)
	}
	want := map[Script]bool{"Hani": true, "Hira": true, ScriptEmoji: true}
	if len(got) != len(want) {
		t.Fatalf("reported %v, want one entry per script in %v", got, want)
	}
	for _, sc := range got {
		if !want[sc] {
			t.Errorf("unexpected script %q in %v", sc, got)
		}
	}
}

// A chain that does cover the text must stay silent: this is the common path
// and a report on it would be noise.
func TestCoveredTextReportsNothing(t *testing.T) {
	s := NewShaper(regular(t))
	fired := 0
	s.OnMissingGlyph(func(rune, Script) { fired++ })
	s.Line("Hello, world — Ωmega, Привет", 16)
	if fired != 0 {
		t.Errorf("covered text reported %d missing glyphs", fired)
	}
}

func TestOnMissingGlyphNilSilences(t *testing.T) {
	s := NewShaper(regular(t))
	s.OnMissingGlyph(nil)
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	s.Line("中", 16)
	if buf.Len() != 0 {
		t.Errorf("OnMissingGlyph(nil) still logged: %s", buf.String())
	}
}

// Unconfigured means "use the default report", which has to be distinguishable
// from an explicit nil — a bare nil func cannot tell those apart, which is what
// the missingSet flag is for.
func TestDefaultReportLogs(t *testing.T) {
	s := NewShaper(regular(t))
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	s.Line("中", 16)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("U+4E2D")) {
		t.Errorf("default report did not name the rune: %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Hani")) {
		t.Errorf("default report did not name the script: %q", out)
	}
	// The report is only useful if it says what to do about it.
	if !bytes.Contains(buf.Bytes(), []byte("UseWebFonts")) {
		t.Errorf("default report offered no remedy: %q", out)
	}
}

func TestInvisibleRunesAreNotReported(t *testing.T) {
	s := NewShaper(regular(t))
	fired := 0
	s.OnMissingGlyph(func(rune, Script) { fired++ })
	// Called directly: whether Go Regular happens to carry a zero-width
	// space is not the point, and routing through Line would make the test
	// depend on it.
	for _, r := range []rune{0x200B, 0x200D, 0xFE0F, 0xFEFF, '\n'} {
		s.reportMissing(r)
	}
	if fired != 0 {
		t.Errorf("invisible runes produced %d reports", fired)
	}
}

// A stream of mojibake must not become the thing that makes the app unusable.
func TestMissingReportIsCapped(t *testing.T) {
	s := NewShaper(regular(t))
	fired := 0
	s.OnMissingGlyph(func(rune, Script) { fired++ })
	// One rune from each of many scripts, well past the cap.
	for _, r := range []rune{
		'中', 'あ', 'カ', '한', 'ع', 'א', 'ह', 'ก', '😀', 'ก', 'ᑌ', 'ᚠ', 'ꜩ', 'ⴰ', 'ߒ', 'ހ', 'ⲁ', 'ᐁ',
	} {
		s.reportMissing(r)
	}
	if fired > missingScriptCap {
		t.Errorf("reported %d scripts, cap is %d", fired, missingScriptCap)
	}
	if fired == 0 {
		t.Error("the cap silenced everything")
	}
}
