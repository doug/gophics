//go:build !js

package text

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// emojiRune is the one every platform that has any emoji font has.
const emojiRune = '\U0001F600' // GRINNING FACE

// emojiShaper returns a Shaper over the Go regular face extended with the
// machine's installed fonts, plus the glyph the emoji in "hi 😀 there" shaped
// to. It capability-skips rather than failing when the machine has no system
// fonts or no colour-bitmap emoji font: a CI container typically has neither,
// and Windows has a COLR font instead, which this route deliberately does not
// render (see coloremoji.go).
func emojiShaper(t *testing.T) (*Shaper, Glyph) {
	t.Helper()
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	s := NewShaper(f)
	if err := s.UseSystemFonts(t.TempDir()); err != nil {
		t.Skipf("capability skip: no system fonts on this machine: %v", err)
	}
	l := s.Line("hi \U0001F600 there", 32)
	for _, g := range l.Glyphs {
		if g.Font == s.Primary() {
			continue
		}
		if _, ok := g.Font.ColorGlyphBox(g.GID, 32); ok {
			return s, g
		}
	}
	t.Skip("capability skip: no colour-bitmap emoji font installed " +
		"(a COLR-only font such as Segoe UI Emoji is expected to land here)")
	return nil, Glyph{}
}

// TestFallbackReachesColorEmojiFace is the first of the four things that have
// to work: before the script hint went in, fontscan answered U+1F600 with an
// arbitrary face that has no glyph for it, so shaping produced GID 0 from the
// Latin font and no amount of bitmap support downstream could have helped.
func TestFallbackReachesColorEmojiFace(t *testing.T) {
	s, g := emojiShaper(t)
	if g.Font == s.Primary() {
		t.Fatal("emoji resolved to the primary font")
	}
	if g.GID == 0 {
		t.Fatal("emoji shaped to GID 0 (.notdef): fallback did not reach a face that covers it")
	}
	if !g.Font.HasGlyph(emojiRune) {
		t.Fatal("the resolved face has no glyph for the emoji")
	}
	// Setting the script must not cost Latin its own font. The explicit chain
	// is consulted first, so this also pins that ordering.
	for _, lg := range s.Line("hi", 32).Glyphs {
		if lg.Font != s.Primary() {
			t.Fatal("Latin no longer resolves to the primary font")
		}
	}
}

// TestColorGlyphImageDecodes covers the accessor: the font library locates the
// bitmap but hands back raw PNG bytes, so this is also the test that the
// decode happens at all.
func TestColorGlyphImageDecodes(t *testing.T) {
	_, g := emojiShaper(t)
	box, ok := g.Font.ColorGlyphBox(g.GID, 32)
	if !ok {
		t.Fatal("ColorGlyphBox: not a colour glyph")
	}
	if box.Dx() <= 0 || box.Dy() <= 0 {
		t.Fatalf("ColorGlyphBox: empty box %+v", box)
	}
	// y-down with the pen on the baseline: the glyph sits above it.
	if box.Min.Y >= 0 {
		t.Fatalf("ColorGlyphBox: box does not rise above the baseline: %+v", box)
	}
	ppem := g.Font.ColorGlyphPpem(32)
	if ppem <= 0 {
		t.Fatal("ColorGlyphPpem: no bitmap strikes")
	}
	img, ok := g.Font.ColorGlyphImage(g.GID, ppem)
	if !ok {
		t.Fatal("ColorGlyphImage: no image")
	}
	if img.Bounds().Empty() {
		t.Fatal("ColorGlyphImage: empty bounds")
	}
	// It must be *colour*, not a coverage mask dressed up as one: look for a
	// pixel whose channels are not all equal. A monochrome glyph could pass
	// every other assertion here.
	b := img.Bounds()
	colored := false
	for y := b.Min.Y; y < b.Max.Y && !colored; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, gg, bb, a := img.At(x, y).RGBA()
			if a > 0 && (r != gg || gg != bb) {
				colored = true
				break
			}
		}
	}
	if !colored {
		t.Error("decoded emoji bitmap has no coloured pixel")
	}
}

// TestColorGlyphImageRejectsOutlines pins the negative answer. An outline
// glyph must not come back as a blittable bitmap, or every Latin glyph would
// be routed through the atlas and drawn as nothing.
func TestColorGlyphImageRejectsOutlines(t *testing.T) {
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := f.NominalGID('A')
	if !ok || gid == 0 {
		t.Fatal("no glyph for 'A'")
	}
	if _, ok := f.ColorGlyphBox(gid, 20); ok {
		t.Error("ColorGlyphBox said an outline glyph is a colour bitmap")
	}
	if p := f.ColorGlyphPpem(20); p != 0 {
		t.Errorf("ColorGlyphPpem = %d for a font with no bitmap strikes, want 0", p)
	}
	if _, ok := f.ColorGlyphImage(gid, 20); ok {
		t.Error("ColorGlyphImage decoded an outline glyph")
	}
	// COLR is the case this route deliberately does not handle, and no COLR
	// font is reachable from a pure-Go test fixture here, so it is covered by
	// construction rather than by assertion: ColorGlyphImage asks
	// GlyphDataColor first and returns false when it answers. If a COLR
	// fixture ever lands in internal/testfont, assert it here.
}

// TestColorGlyphPpemPicksAStrike is the regression test for the ppem-0 decode.
// A face gophics never told a size to reports ppem 0, and the font library then
// hands back its *largest* strike: measured at 160x160 and 23,780 bytes of PNG
// for Apple Color Emoji where the 20-ppem strike is 20x20 and 1,093 bytes. That
// is both a 20x decode and an 8x minification through a fixed bilinear sampler.
func TestColorGlyphPpemPicksAStrike(t *testing.T) {
	_, g := emojiShaper(t)
	small, ok := g.Font.ColorGlyphImage(g.GID, g.Font.ColorGlyphPpem(20))
	if !ok {
		t.Fatal("no image at 20px")
	}
	big, ok := g.Font.ColorGlyphImage(g.GID, g.Font.ColorGlyphPpem(120))
	if !ok {
		t.Fatal("no image at 120px")
	}
	t.Logf("20px -> ppem %d, %v; 120px -> ppem %d, %v",
		g.Font.ColorGlyphPpem(20), small.Bounds(), g.Font.ColorGlyphPpem(120), big.Bounds())
	if small.Bounds().Dx() >= big.Bounds().Dx() {
		t.Errorf("strike selection does not follow size: 20px gave %v, 120px gave %v",
			small.Bounds(), big.Bounds())
	}

	// Quantisation must be idempotent, which is what makes it usable as an
	// atlas key: feeding the chosen ppem back in has to choose the same
	// strike, or a zoom animation mints a cell per frame.
	for _, px := range []int{1, 7, 20, 21, 33, 64, 100, 400} {
		p := g.Font.ColorGlyphPpem(px)
		if q := g.Font.ColorGlyphPpem(p); q != p {
			t.Errorf("ColorGlyphPpem(%d) = %d, but ColorGlyphPpem(%d) = %d: not idempotent", px, p, p, q)
		}
	}
}

// TestColorGlyphBoxScalesWithSize pins that the box is in logical pixels at the
// requested size, not in font units and not in the strike's pixels.
func TestColorGlyphBoxScalesWithSize(t *testing.T) {
	_, g := emojiShaper(t)
	a, ok := g.Font.ColorGlyphBox(g.GID, 16)
	if !ok {
		t.Fatal("not a colour glyph at 16")
	}
	b, ok := g.Font.ColorGlyphBox(g.GID, 32)
	if !ok {
		t.Fatal("not a colour glyph at 32")
	}
	if got, want := b.Dy(), a.Dy()*2; got < want*0.99 || got > want*1.01 {
		t.Errorf("box height at 32 = %g, want ~%g (twice the height at 16)", got, want)
	}
}
