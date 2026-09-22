package ui

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Icons -------------------------------------------------------------------

// iconGlyphs is every theme.Icon glyph with its name, in constant order. It is
// a list rather than a loop to the last constant because the sheet exists to
// be read: a glyph that nobody named is a glyph nobody will find.
var iconGlyphs = []struct {
	glyph theme.IconGlyph
	name  string
}{
	{theme.IconHome, "Home"}, {theme.IconList, "List"}, {theme.IconSearch, "Search"},
	{theme.IconSliders, "Sliders"}, {theme.IconChevronLeft, "ChevronLeft"},
	{theme.IconChevronRight, "ChevronRight"}, {theme.IconChevronUp, "ChevronUp"},
	{theme.IconChevronDown, "ChevronDown"}, {theme.IconPlus, "Plus"}, {theme.IconClose, "Close"},
	{theme.IconCheck, "Check"}, {theme.IconMenu, "Menu"}, {theme.IconCalendar, "Calendar"},
	{theme.IconChart, "Chart"}, {theme.IconBell, "Bell"}, {theme.IconPeople, "People"},
	{theme.IconPerson, "Person"}, {theme.IconPuzzle, "Puzzle"}, {theme.IconGrip, "Grip"},
	{theme.IconReply, "Reply"}, {theme.IconCheckCircle, "CheckCircle"}, {theme.IconTrash, "Trash"},
	{theme.IconShare, "Share"}, {theme.IconCopy, "Copy"},
}

// iconsSection is the glyph sheet: every Icon at three sizes, named, so a
// shape that lost weight at small sizes or drifted off its grid is seen here
// before it is seen in a nav bar.
type iconsSection struct{}

func (iconsSection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	tiles := make([]widget.Widget, 0, len(iconGlyphs))
	for _, g := range iconGlyphs {
		col := widget.Column(
			theme.Icon{Glyph: g.glyph, Size: 28},
			widget.Sized{H: 6},
			widget.Text{Value: g.name, Size: th.Type.Caption, Color: th.Muted},
		)
		col.CrossAlign = layout.CrossCenter
		tiles = append(tiles, widget.Sized{W: 88, Child: col})
	}
	// The same glyphs at text size and in the accent colour: an icon spends
	// most of its life beside a label, so that is the pairing to judge.
	inline := make([]widget.Widget, 0, len(iconGlyphs))
	for _, g := range iconGlyphs {
		inline = append(inline, theme.Icon{Glyph: g.glyph, Color: th.Primary})
	}
	return sectionColumn(
		groupLabel("Every glyph"),
		theme.Body("Stroked paths on a 24×24 grid — no icon font to go missing."),
		widget.Sized{H: 8},
		widget.Wrap{Spacing: 4, RunSpacing: 16, Children: tiles},
		groupLabel("At body size"),
		widget.Padding{Insets: geom.InsetsSymmetric(0, 4), Child: widget.Wrap{Spacing: 10, RunSpacing: 10, Children: inline}},
	)
}
