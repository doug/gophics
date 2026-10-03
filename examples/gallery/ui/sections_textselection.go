package ui

import (
	"fmt"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// textSelectionSection is the one place in the catalog that answers "can I
// select this text and copy it?" — the question a reader asks of any app,
// usually by trying it rather than by reading about it.
//
// So the demo is the text itself: a real passage worth lifting a quote out of,
// three separate widgets (heading, body, attribution) inside one SelectionArea
// so a drag that crosses them selects one continuous range the way a browser
// does. Everything else on the page exists to make the result visible — a live
// readout of what is selected, and a field to paste into, because a copy you
// cannot paste is not a demonstration of copying.
type textSelectionSection struct{}

func (textSelectionSection) CreateState() widget.State { return &textSelectionDemo{} }

type textSelectionDemo struct {
	widget.StateBase[textSelectionSection]
	selected string
	pasted   string
}

// The passage is deliberately a few lines of ordinary prose with punctuation
// and a proper noun in it: the things word selection gets wrong are hyphens,
// full stops and capitals, and a demo made of "lorem ipsum" would hide them.
const (
	selTitle = "On the pleasure of selecting text"
	selBody  = "Selection is the quietest part of an interface, and the first " +
		"thing a reader reaches for. It should take a word on a double click, " +
		"a paragraph on a third, and give the whole thing up to Select All — " +
		"without ever leaving half a word behind while the pointer moves."
	selAttrib = "— the gophics gallery, 2026"
)

func (s *textSelectionDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	cmd := "Ctrl"
	if ctx.MacKeys() {
		cmd = "Cmd"
	}

	passage := widget.Column(
		widget.Text{Value: selTitle, Font: theme.FontBold, Size: th.Type.Title, Color: th.Text, Wrap: true},
		widget.Sized{H: 8},
		widget.Text{Value: selBody, Size: th.Type.Body, Color: th.Text, Wrap: true},
		widget.Sized{H: 8},
		widget.Text{Value: selAttrib, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	)
	passage.CrossAlign = layout.CrossStart

	return sectionColumn(
		groupLabel("Drag across any of it — the three blocks select as one"),
		// The SelectionArea wraps the padding, not the other way round, and the
		// card contributes none of its own (Pad: 0): a drag that starts in the
		// margin should select the nearest line, as it does in a browser, and
		// an area drawn tightly around the glyphs would never see that press.
		theme.Card{Pad: 0, Child: widget.SelectionArea{
			Child: widget.Padding{
				Insets: geom.Insets{Top: 14, Bottom: 14, Left: 14, Right: 14},
				Child:  passage,
			},
			OnSelect: func(t string) { s.SetState(func() { s.selected = t }) },
		}},

		groupLabel("What is selected, as you select it"),
		widget.Text{Value: selectionReadout(s.selected), Size: th.Type.Caption, Color: th.Muted, Wrap: true},

		groupLabel("Copy it, then paste it back"),
		theme.Field{
			Value:       s.pasted,
			Placeholder: "Paste here (" + cmd + "+V, or long-press → Paste)",
			OnChange:    func(v string) { s.SetState(func() { s.pasted = v }) },
		},

		groupLabel("What to try"),
		hintList(th, []string{
			"Double-click a word, then drag — it keeps selecting whole words.",
			"Triple-click takes the whole paragraph.",
			"Click once, then shift-click further along to extend.",
			cmd + "+A selects all three blocks; " + cmd + "+C copies.",
			"Right-click without a selection takes the word under the pointer.",
			"On a phone: long-press a word, drag the grips to adjust, then Copy.",
		}),
	)
}

// selectionReadout describes the selection in the terms a reader cares about —
// how much, and the text itself, elided in the middle so both ends stay
// visible. The ends are what tell you whether the selection stopped where you
// meant it to.
func selectionReadout(sel string) string {
	if sel == "" {
		return "Nothing selected yet."
	}
	n := len([]rune(sel))
	shown := strings.ReplaceAll(sel, "\n", " ⏎ ")
	if r := []rune(shown); len(r) > 72 {
		shown = string(r[:34]) + " … " + string(r[len(r)-34:])
	}
	unit := "characters"
	if n == 1 {
		unit = "character"
	}
	return fmt.Sprintf("%d %s selected:  %q", n, unit, shown)
}

// hintList renders the per-platform notes as a plain bulleted column. They are
// inside no SelectionArea on purpose: the one above should be the only text on
// the page that highlights, so it is obvious which part is the demo.
func hintList(th theme.Theme, lines []string) widget.Widget {
	kids := make([]widget.Widget, 0, len(lines)*2)
	for i, l := range lines {
		if i > 0 {
			kids = append(kids, widget.Sized{H: 4})
		}
		row := widget.Row(
			widget.Text{Value: "·  ", Size: th.Type.Caption, Color: th.Muted},
			widget.Flexible{Child: widget.Text{Value: l, Size: th.Type.Caption, Color: th.Muted, Wrap: true}},
		)
		row.CrossAlign = layout.CrossStart
		kids = append(kids, row)
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return col
}
