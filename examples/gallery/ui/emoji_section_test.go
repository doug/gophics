package ui

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

// The claim this section makes is not "emoji render" — it cannot promise that,
// because the font chain decides. The claim is that it never shows tofu: with
// no colour face reachable it draws words explaining why, and it does not draw
// the emoji row at all.
//
// Drawn text rather than labels is what has to be asserted here. The rows go
// onto a paint.Canvas, so they are display-list text runs and never appear in
// the semantics tree — a.Labels() would pass while the canvas drew eight boxes.
func TestEmojiSectionFallsBackToWordsWithNoColorFace(t *testing.T) {
	a := apptest.New(t, emojiSection{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
		// Deliberately no SystemFonts: this is the Latin-only chain every CI
		// container has, and the case the fallback exists for.
	}))
	a.Render()

	drawn := strings.Join(a.Scene().Texts(), "\x00")
	if !strings.Contains(drawn, "paint.Emoji reported no colour face") {
		t.Errorf("the fallback note was not drawn; drawn runs: %q", a.Scene().Texts())
	}
	for _, e := range emojiRow {
		if strings.Contains(drawn, e) {
			t.Errorf("drew %q with no colour face reachable: that is a tofu box", e)
		}
	}
}

// The positive half: with the machine's fonts in the chain the same section
// draws the sheet instead of the note. Capability-skipped, for the same reason
// every emoji test in paint is — a CI container has no system fonts, and a
// stock Windows build's emoji font is COLR, which this module does not draw.
func TestEmojiSectionDrawsTheSheetWhenAFaceIsThere(t *testing.T) {
	a := apptest.New(t, emojiSection{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF, SystemFonts: true,
	}))
	a.Render()

	drawn := strings.Join(a.Scene().Texts(), "\x00")
	if strings.Contains(drawn, "paint.Emoji reported no colour face") {
		t.Skip("capability skip: no colour-bitmap emoji face reachable on this machine")
	}
	for _, e := range emojiRow {
		if !strings.Contains(drawn, e) {
			t.Errorf("a colour face is present but %q was never drawn", e)
		}
	}
	if !strings.Contains(drawn, "one glyph, four sizes") {
		t.Error("the strike row's caption is missing, so the third row did not draw")
	}
}
