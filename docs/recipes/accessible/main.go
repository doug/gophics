// Command accessible is the cookbook's accessibility recipe: a control that is
// drawn rather than composed, made operable and legible to assistive
// technology, and a decorative graphic hidden from it.
//
//	go run ./docs/recipes/accessible
//	go test ./docs/recipes/accessible
//
// The framework derives semantics from the tree it already has: a Text
// contributes its string, an Interactive with OnTap contributes a button. That
// covers most screens. It cannot cover a control painted inside a Canvas,
// because there is nothing in the tree to read — which is what Sem and
// Semantics are for.
//
// The test beside this file is the point as much as the program is: semantics
// are an ordinary value under `go test`, with no screen reader, no emulator
// and no device. That is the cheapest accessibility check any GUI stack
// offers, and it is worth putting in CI on day one rather than before a
// procurement review.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

const stars = 5

type Rating struct{}

func (Rating) CreateState() widget.State { return &ratingState{} }

type ratingState struct {
	widget.StateBase[Rating]
	picked int // 0 = unrated
}

func (s *ratingState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	kids := make([]widget.Widget, 0, stars)
	for i := 1; i <= stars; i++ {
		n, on := i, i <= s.picked
		kids = append(kids, widget.Interactive{
			// Sem, not a Semantics wrapper around the Interactive. Both work,
			// but Sem keeps the control as one node: a radio nested inside a
			// button is worse for a screen-reader user than either alone,
			// because it has to be stepped through twice.
			//
			// Label is required here. Without it this node is a button with
			// no name — announced as "button", which tells the user nothing.
			// OnActivate defaults to Gestures.OnTap, so the control is
			// operable by a VoiceOver double-tap without saying so twice.
			Sem: &layout.SemInfo{
				Role:    layout.RoleRadio,
				Label:   fmt.Sprintf("%d of %d stars", n, stars),
				Checked: &on,
			},
			Gestures: widget.Gestures{
				OnTap: func() { s.SetState(func() { s.picked = n }) },
			},
			// theme.MinTouchTarget is the platform minimum (44pt). A glyph
			// drawn at 20px is a 20px tap target unless something says
			// otherwise, and that is the single most common accessibility
			// defect in a hand-drawn control.
			Child: widget.Sized{W: theme.MinTouchTarget, H: theme.MinTouchTarget,
				Child: widget.Center(star(th, on))},
		})
	}

	row := widget.Row(kids...)
	row.CrossAlign = layout.CrossCenter

	col := widget.Column(
		// A heading is a landmark: it is how a screen-reader user skims a
		// page instead of reading all of it.
		widget.Semantics{Role: layout.RoleHeading, Label: "Rate this recipe",
			Child: theme.Title("Rate this recipe")},
		widget.Sized{H: 8},

		// Hidden removes a subtree from the semantics tree entirely. A
		// decorative rule carries no information, and announcing it is noise.
		widget.Semantics{Hidden: true, Child: widget.Canvas{H: 6, Draw: func(c paint.Canvas, size geom.Size) {
			c.FillRRect(geom.RectXYWH(0, 2, size.W, 2), 1, th.Border)
		}}},

		widget.Sized{H: 14},
		row,
		widget.Sized{H: 14},

		// Live text is announced when it changes, so the status line is also
		// the confirmation that a tap did something.
		widget.Text{Value: statusLine(s.picked), Size: th.Type.Body, Color: th.Muted},
	)
	col.CrossAlign = layout.CrossStart

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 20, Child: col}}}
}

func statusLine(n int) string {
	if n == 0 {
		return "Not rated yet"
	}
	return fmt.Sprintf("Rated %d of %d", n, stars)
}

// star is the drawn part. Nothing here contributes semantics — that is the
// whole reason the Interactive above has to declare them.
func star(th theme.Theme, on bool) widget.Widget {
	col := th.Outline
	if on {
		col = th.Primary
	}
	return widget.Canvas{W: 22, H: 22, Draw: func(c paint.Canvas, size geom.Size) {
		c.FillRRect(geom.RectXYWH(3, 3, size.W-6, size.H-6), 3, col)
	}}
}

func main() {
	if err := app.Run(Rating{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · accessibility",
		Size:           geom.Size{W: 420, H: 280},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
