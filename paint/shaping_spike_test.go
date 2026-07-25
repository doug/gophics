package paint

import (
	"image/png"
	"os"
	"runtime"
	"testing"

	"github.com/gogpu/gg/text"

	"github.com/doug/gossamer/geom"
)

// TestArabicShapingSpike probes whether gg's from-scratch GSUB engine
// actually applies contextual (positional) shaping to Arabic — the plan's
// standing skepticism about gg's text stack (PLAN.md §5.1). It is an
// ecosystem spike, not a framework test: it uses a macOS system font and
// skips elsewhere.
//
// Two signals, either sufficient:
//  1. glyph IDs for a joined word differ from the isolated-form glyphs of
//     the same runes (positional forms were substituted), and
//  2. the shaped advance differs from the sum of isolated-rune advances.
func TestArabicShapingSpike(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("spike uses a macOS system font")
	}
	const fontPath = "/System/Library/Fonts/SFArabic.ttf"
	if _, err := os.Stat(fontPath); err != nil {
		t.Skip("SFArabic.ttf not present")
	}
	src, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("font parse failed: %v", err)
	}
	face := src.Face(28)

	const word = "السلام" // joined Arabic word
	shaped := face.AppendGlyphs(nil, word)
	if len(shaped) == 0 {
		t.Fatal("no glyphs shaped")
	}

	var isolated []text.Glyph
	var isolatedAdvance float64
	for _, r := range word {
		g := face.AppendGlyphs(nil, string(r))
		isolated = append(isolated, g...)
		isolatedAdvance += face.Advance(string(r))
	}

	shapedIDs := map[text.GlyphID]bool{}
	for _, g := range shaped {
		shapedIDs[g.GID] = true
	}
	positional := 0
	for _, g := range isolated {
		if !shapedIDs[g.GID] {
			positional++
		}
	}
	t.Logf("Face path: shaped glyphs=%d isolated glyphs=%d substituted=%d faceAdv=%.1f isolatedAdv=%.1f",
		len(shaped), len(isolated), positional, face.Advance(word), isolatedAdvance)

	// The real shaper (what DrawString uses).
	sg := text.Shape(word, face)
	shaperIDs := map[text.GlyphID]bool{}
	var shaperAdvance float64
	for _, g := range sg {
		shaperIDs[g.GID] = true
		shaperAdvance += g.XAdvance
	}
	substituted := 0
	for _, g := range shaped {
		if !shaperIDs[g.GID] {
			substituted++
		}
	}
	t.Logf("Shape() path: glyphs=%d positional-substituted=%d shaperAdv=%.1f",
		len(sg), substituted, shaperAdvance)

	if substituted == 0 {
		// Verdict as of gg v0.50.x, recorded 2026-07-24: no Arabic contextual
		// joining in OwnShaper (and no bidi anywhere). Latin-only until the
		// go-text/typesetting-based text package lands (PLAN.md §6.1). This
		// skip flips to a hard assertion when that happens.
		t.Skip("FINDING CONFIRMED: gg applies no Arabic positional substitution; " +
			"complex scripts require go-text/typesetting (PLAN.md §5.1)")
	}
	if diff := shaperAdvance - face.Advance(word); diff > 0.5 || diff < -0.5 {
		t.Errorf("LAYOUT/PAINT MISMATCH: Face.Advance=%.1f (unshaped, used by layout) "+
			"vs Shape() advance=%.1f (used by DrawString) — measurement must go "+
			"through the shaper for shaped scripts", face.Advance(word), shaperAdvance)
	}

	if out := os.Getenv("GOSSAMER_RENDER_OUT"); out != "" {
		p := NewPainter()
		data, err := os.ReadFile(fontPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.LoadFont(data); err != nil {
			t.Fatal(err)
		}
		c := p.BeginOffscreen(geom.Size{W: 420, H: 120}, 2)
		c.Clear(RGB(1, 1, 1))
		c.Text("السلام عليكم", geom.Pt{X: 20, Y: 60}, 32, RGB(0, 0, 0))
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, p.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
