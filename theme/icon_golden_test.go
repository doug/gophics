package theme_test

import (
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// The whole glyph set, drawn. TestEveryGlyphDrawsInsideItsBox proves each
// path is non-empty and on its grid; this is the check that they look like
// what they are named, which only a picture can carry. Review the image when
// it changes.
func TestIconSheetGolden(t *testing.T) {
	glyphs := []theme.IconGlyph{
		theme.IconHome, theme.IconList, theme.IconSearch, theme.IconSliders,
		theme.IconChevronLeft, theme.IconChevronRight, theme.IconChevronUp, theme.IconChevronDown,
		theme.IconPlus, theme.IconClose, theme.IconCheck, theme.IconMenu,
		theme.IconCalendar, theme.IconChart, theme.IconBell, theme.IconPeople,
		theme.IconPerson, theme.IconPuzzle, theme.IconGrip, theme.IconReply,
		theme.IconCheckCircle, theme.IconTrash, theme.IconShare, theme.IconCopy,
	}
	if last := theme.IconCopy; int(last)+1 != len(glyphs) {
		t.Fatalf("sheet lists %d glyphs but the enum has %d — add the new one here too", len(glyphs), int(last)+1)
	}
	const per = 6
	var rows []widget.Widget
	for i := 0; i < len(glyphs); i += per {
		var cells []widget.Widget
		for _, g := range glyphs[i:min(i+per, len(glyphs))] {
			cells = append(cells, widget.Padding{
				Insets: geom.InsetsAll(8),
				Child:  theme.Icon{Glyph: g, Size: 32},
			})
		}
		rows = append(rows, widget.Row(cells...))
	}
	a := apptest.New(t, widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Column(rows...)},
		apptest.Size(300, 200), apptest.Scale(2), apptest.Tol(apptest.AntiAliased))
	a.Golden("icons")
}
