// Command themed is the cookbook's theming recipe: a brand palette derived
// from the built-in light and dark themes, provided to the tree, and read back
// by every themed component below it.
//
//	go run ./docs/recipes/themed
//
// A theme is a plain struct of design tokens — colours, a corner radius and a
// type scale. There is no theme registry, no inheritance and no cascade: the
// nearest Provide above a widget wins, which is the one rule.
//
// Derive rather than construct. theme.Theme has around twenty colour fields,
// and the two that catch people out are Border (decorative chrome — dividers,
// a panel's hairline) and Outline (the edge of an interactive control). A
// theme written field by field for pretty panel edges erases its own
// checkboxes; the built-in glass presets did exactly that.
package main

import (
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

// brand is the whole customisation: start from a built-in theme and move the
// tokens that are actually yours.
func brand(dark bool) theme.Theme {
	t := theme.Light()
	if dark {
		t = theme.Dark()
	}
	t.Primary = paint.RGB(0.25, 0.47, 0.84)
	t.OnPrimary = paint.RGB(1, 1, 1)
	t.Radius = 14
	t.Type.Body = 15
	t.Type.Heading = 19
	return t
}

type App struct{}

func (App) CreateState() widget.State { return &appState{} }

type appState struct {
	widget.StateBase[App]
	// override is nil to follow the platform, or a pinned choice. An app with
	// a theme setting needs all three states — light, dark, follow — and a
	// bare bool cannot express the third.
	override *bool
}

func (s *appState) Build(ctx widget.Ctx) widget.Widget {
	// ctx.DarkMode() is the platform's current colour scheme, and it changes
	// while the app runs: the subtree under this Provide rebuilds when it
	// does. That is why a derived value like a theme belongs in the tree and
	// not in app.Config.Provide, which is for things built once at startup.
	dark := ctx.DarkMode()
	if s.override != nil {
		dark = *s.override
	}
	th := brand(dark)

	label := "Following the system"
	switch {
	case s.override != nil && *s.override:
		label = "Pinned dark"
	case s.override != nil:
		label = "Pinned light"
	}

	col := widget.Column(
		theme.Heading("Themed components"),
		widget.Sized{H: 4},
		theme.Caption("every colour, radius and text size below comes from the theme"),
		widget.Sized{H: 16},

		// Nothing here names a colour. theme.Of(ctx) inside each component
		// finds the Provide below, so re-theming the app is one struct change
		// rather than a sweep through the tree.
		theme.Card{Child: widget.Column(
			theme.Button{Label: "Primary action", Primary: true, OnTap: func() {}},
			widget.Sized{H: 10},
			theme.Button{Label: "Secondary", OnTap: func() {}},
			widget.Sized{H: 10},
			theme.Checkbox{Label: "Outline is a different token from Border", Checked: true},
			widget.Sized{H: 10},
			theme.Field{Placeholder: "A themed field"},
		)},

		widget.Sized{H: 18},
		theme.Segmented{
			Options:  []string{"Light", "Dark", "System"},
			Selected: selectedIndex(s.override),
			OnChange: func(i int) {
				s.SetState(func() {
					switch i {
					case 0:
						s.override = boolPtr(false)
					case 1:
						s.override = boolPtr(true)
					default:
						s.override = nil
					}
				})
			},
		},
		widget.Sized{H: 8},
		theme.Caption(label),
	)
	col.CrossAlign = layout.CrossStretch

	// Provide sits above everything that reads the theme, including the Fill
	// that paints the background. Put it below the Fill and the page keeps the
	// old background for one frame after a scheme change.
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 20, Child: col}}}
}

func selectedIndex(override *bool) int {
	switch {
	case override == nil:
		return 2
	case *override:
		return 1
	default:
		return 0
	}
}

func boolPtr(b bool) *bool { return &b }

func main() {
	if err := app.Run(App{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title: "recipe · theming",
		Size:  geom.Size{W: 440, H: 560},
		// Background and BackgroundDark go together. The window background is
		// resolved per frame from the same colour-scheme signal the theme is,
		// so a single fixed colour here renders dark-theme text over a light
		// page the moment the viewer prefers dark — and nothing errors.
		Background:     brand(false).Bg,
		BackgroundDark: brand(true).Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
