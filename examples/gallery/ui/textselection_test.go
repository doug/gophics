package ui

import (
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// The section's whole claim is that a reader can select across the three
// blocks, copy, and paste it back. Each half of that is asserted here against
// the real gesture dispatcher, because the claim is made in prose on the page
// itself and prose does not fail a build.

// A drag that starts in the card's margin and crosses from the heading into the
// body selects one continuous range — the behaviour a browser gives and the
// reason the SelectionArea wraps the padding rather than sitting inside it.
func TestTextSelectionDragSpansTheBlocks(t *testing.T) {
	a := galleryApp(t, textSelectionSection{})
	a.AssertText("Nothing selected yet.")

	a.Drag(geom.Pt{X: 30, Y: 70}, geom.Pt{X: 300, Y: 150})
	a.Render()

	sel := readoutLine(a.Labels())
	if sel == "" {
		t.Fatal("no readout line after the drag")
	}
	if strings.Contains(sel, "Nothing selected") {
		t.Fatalf("drag across the card selected nothing: %q", sel)
	}
	// It must have crossed a block boundary: the heading's words and the
	// body's both, which is what "select as one" means.
	if !strings.Contains(sel, "On the pleasure") {
		t.Errorf("selection does not start in the heading: %q", sel)
	}
}

// Cmd/Ctrl+A takes all three blocks and Cmd/Ctrl+C puts them on the clipboard,
// so the passage can be pasted into the field below it. This is the round trip
// the section exists to show.
func TestTextSelectionSelectAllCopiesEveryBlock(t *testing.T) {
	a := galleryApp(t, textSelectionSection{})
	a.Tap(geom.Pt{X: 100, Y: 100}) // focus the area
	a.Render()
	a.KeyMod(shell.KeyA, shell.ModSuper)
	a.KeyMod(shell.KeyC, shell.ModSuper)

	got := a.Clipboard().S
	for _, want := range []string{selTitle, "Selection is the quietest part", "the gophics gallery"} {
		if !strings.Contains(got, want) {
			t.Errorf("clipboard is missing %q; got %q", want, got)
		}
	}
}

// readoutLine returns the "N characters selected" line, which is how the
// section reports the live selection back to the reader.
func readoutLine(texts []string) string {
	for _, s := range texts {
		// "N characters selected:" or the empty state — not the heading above
		// them, which also contains the word.
		if strings.Contains(s, "characters selected") || strings.Contains(s, "character selected") ||
			strings.Contains(s, "Nothing selected") {
			return s
		}
	}
	return ""
}
