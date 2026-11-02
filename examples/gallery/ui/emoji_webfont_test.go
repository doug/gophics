package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// subsetPath is the face Config.WebFonts serves to the web build, held in the
// docs site because that is where it is fetched from. The test reaches up out
// of the package rather than embedding a second copy: one file, so a re-subset
// cannot update the asset and leave a stale copy passing the test.
var subsetPath = filepath.Join("..", "..", "..", "docs", "fonts", "NotoColorEmoji-gallery-subset.ttf")

// TestGalleryEmojiSubsetIsAColourBitmapFace is the guard on the one property of
// the vendored subset that cannot be read off its filename: that gophics can
// actually get colour pixels out of it.
//
// It matters because the obvious mistake here is silent. Noto Color Emoji is
// published in two builds under one name — a CBDT/CBLC bitmap face, and the
// COLRv1 face that fonts.google.com serves. gophics rasterizes colour bitmaps
// and not COLR paint graphs, so swapping one for the other leaves a font that
// fetches with no error, reports no error, and draws nothing. No test of the
// URL, the config, or the fetch would notice; only asking for a glyph does.
//
// Unlike the other emoji tests in this package, this one is not
// capability-skipped. It loads no system font and needs no GPU — the face is in
// the repo, so if a colour glyph does not come back, the asset is wrong.
func TestGalleryEmojiSubsetIsAColourBitmapFace(t *testing.T) {
	face, err := os.ReadFile(subsetPath)
	if err != nil {
		t.Fatalf("reading the subset the web build serves: %v\n"+
			"Config.WebFonts points at /fonts/NotoColorEmoji-gallery-subset.ttf; "+
			"if it moved, docs/fonts/README.md and gallery.go both have to follow", err)
	}

	// The chain the web build runs once the fetch lands: the bundled Latin face
	// the catalog ships with, and the fetched emoji face behind it. Loaded here
	// rather than through Config.WebFonts because WebFonts is a no-op off js —
	// the bytes are what is under test, not the transport.
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if err := p.LoadFallbackFont(face); err != nil {
		t.Fatalf("the subset would not load as a font at all: %v", err)
	}

	// Painter answers HasColorEmoji itself; paint.Emoji is the capability
	// lookup for a Canvas, which is how the section asks at draw time.
	for _, r := range emojiRow {
		if !p.HasColorEmoji("", r, emojiBig) {
			t.Errorf("no colour glyph for %q from the subset: the face is "+
				"probably a COLR build, which this module does not rasterize — "+
				"see docs/fonts/README.md", r)
		}
	}

	// The negative half, in the same chain: Latin still comes from the Go face
	// and is not reported as colour. A subset that somehow claimed every rune
	// would pass the loop above while telling the section to draw emoji for
	// ordinary text.
	if p.HasColorEmoji("", "abc", emojiBig) {
		t.Error(`HasColorEmoji reported colour glyphs for "abc"`)
	}

	// And the control, so none of the above can pass vacuously: the identical
	// chain without the subset answers false for the same emoji. Without this,
	// a HasColorEmoji that had come to return true for everything would look
	// like a working font.
	//
	// This logs text's "no font in the fallback chain covers U+1F680" warning,
	// which is the control working rather than anything going wrong. Silencing
	// it needs Shaper.OnMissingGlyph, which paint does not expose.
	bare := paint.NewPainter()
	if err := bare.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if bare.HasColorEmoji("", emojiRow[0], emojiBig) {
		t.Errorf("the Latin-only chain reported a colour glyph for %q, so the "+
			"assertions above prove nothing about the subset", emojiRow[0])
	}
}

// TestEmojiSectionDrawsTheSheetFromTheVendoredSubset is the step past the
// capability check: the section itself, rendering with the fetched face as its
// only source of colour glyphs, draws the emoji rows rather than the apology.
//
// This is the sibling of TestEmojiSectionDrawsTheSheetWhenAFaceIsThere with the
// skip removed. That one depends on the machine having an installed colour face
// and is skipped on CI and on Windows, so the section's drawing path had no
// unconditional coverage at all. This face is in the repo, so this one runs
// everywhere — including on the Linux containers where the web build is made.
//
// What it still does not cover is the fetch: Config.WebFonts is a no-op off js,
// so the transport is exercised by text's own webfont_js tests under node, not
// here. This covers everything downstream of the bytes arriving.
func TestEmojiSectionDrawsTheSheetFromTheVendoredSubset(t *testing.T) {
	face, err := os.ReadFile(subsetPath)
	if err != nil {
		t.Fatalf("reading the subset the web build serves: %v", err)
	}

	a := apptest.New(t, emojiSection{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760},
		Font: goregular.TTF,
		// No SystemFonts: the subset is the whole colour chain, which is the
		// state the web build reaches once the fetch lands.
		Fallbacks: [][]byte{face},
	}))
	a.Render()

	drawn := strings.Join(a.Scene().Texts(), "\x00")
	if strings.Contains(drawn, "paint.Emoji reported no colour face") {
		t.Fatalf("the section drew its no-face apology with the subset loaded, "+
			"so the face reached the chain but produced no colour glyph; "+
			"drawn runs: %q", a.Scene().Texts())
	}
	for _, r := range emojiRow {
		if !strings.Contains(drawn, r) {
			t.Errorf("%q was never drawn, though the subset covers it", r)
		}
	}
	if !strings.Contains(drawn, "one glyph, four sizes") {
		t.Error("the strike row's caption is missing, so the third row did not draw")
	}
}
