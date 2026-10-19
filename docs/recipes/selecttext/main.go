// Command selecttext is the cookbook's text-selection recipe: a passage made
// of three separate Text widgets that a drag selects as one continuous range,
// a live readout of what is selected, and a button that copies it.
//
//	go run ./docs/recipes/selecttext
//
// Selection is framework-level, not platform-level: gophics paints the
// highlight, tracks the range and writes to the platform clipboard itself, so
// double-click-for-a-word, triple-click-for-a-paragraph, shift-click to extend
// and Cmd/Ctrl+C behave the same on desktop, web, a phone and a terminal. No
// DOM text and no native text view is involved.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

const (
	title = "Selecting text"
	body  = "Selection is the quietest part of an interface and the first " +
		"thing a reader reaches for. Three Text widgets inside one " +
		"SelectionArea select as a single range, the way a browser does."
	attrib = "— the gophics cookbook"
)

type Reader struct{}

func (Reader) CreateState() widget.State { return &readerState{} }

type readerState struct {
	widget.StateBase[Reader]
	selected string
	note     string
}

func (s *readerState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	passage := widget.Column(
		widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Title, Color: th.Text, Wrap: true},
		widget.Sized{H: 8},
		widget.Text{Value: body, Size: th.Type.Body, Color: th.Text, Wrap: true},
		widget.Sized{H: 8},
		widget.Text{Value: attrib, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	)
	passage.CrossAlign = layout.CrossStart

	// The area wraps the padding rather than sitting inside it. An area drawn
	// tightly around the glyphs never sees a press that begins in the margin,
	// so a drag starting just left of the first line would select nothing —
	// where a browser would extend to the nearest line.
	//
	// Text widgets in the subtree opt in by reading the area's registry from
	// context. Nothing about the children changes.
	selectable := widget.SelectionArea{
		Child: widget.Padding{Insets: geom.InsetsAll(14), Child: passage},
		// OnSelect fires on every change, and with "" when the selection is
		// cleared — which is how a Copy button knows to disable itself.
		OnSelect: func(t string) { s.SetState(func() { s.selected = t }) },
	}

	col := widget.Column(
		theme.Caption("Drag across any of it — the three blocks select as one"),
		widget.Sized{H: 10},
		theme.Card{Pad: 0, Child: selectable},
		widget.Sized{H: 16},
		widget.Text{Value: readout(s.selected), Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 12},
		theme.Button{
			Label:   "Copy selection",
			Primary: s.selected != "",
			OnTap:   func() { s.copy(ctx) },
		},
		widget.Sized{H: 8},
		widget.Text{Value: s.note, Size: th.Type.Caption, Color: th.Muted},
	)
	col.CrossAlign = layout.CrossStretch

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 18, Child: col}}}
}

// copy writes the selection to the platform clipboard. Cmd/Ctrl+C already does
// this without any code; this exists for the app's own Copy affordance.
//
// ctx.Clipboard() is nil before the runner has provided one, and the write can
// fail — a browser refuses a clipboard write that is not tied to a user
// gesture. Both are ordinary conditions to report, not to panic on.
func (s *readerState) copy(ctx widget.Ctx) {
	if s.selected == "" {
		return
	}
	cb := ctx.Clipboard()
	if cb == nil {
		s.SetState(func() { s.note = "no clipboard on this platform" })
		return
	}
	if err := cb.ClipboardWrite(s.selected); err != nil {
		s.SetState(func() { s.note = "copy refused: " + err.Error() })
		return
	}
	s.SetState(func() { s.note = fmt.Sprintf("copied %d characters", len([]rune(s.selected))) })
}

func readout(sel string) string {
	if sel == "" {
		return "Nothing selected yet."
	}
	n := len([]rune(sel))
	shown := sel
	if r := []rune(shown); len(r) > 64 {
		shown = string(r[:30]) + " … " + string(r[len(r)-30:])
	}
	return fmt.Sprintf("%d characters selected: %q", n, shown)
}

func main() {
	if err := app.Run(Reader{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · select and copy",
		Size:           geom.Size{W: 460, H: 520},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
