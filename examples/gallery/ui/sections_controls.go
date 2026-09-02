package ui

import (
	"fmt"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Buttons & tappables -----------------------------------------------------

// buttonsSection shows the themed Button (default + Primary) and Tappable rows,
// with a live counter proving each tap fires.
type buttonsSection struct{}

func (buttonsSection) CreateState() widget.State { return &buttonsState{} }

type buttonsState struct {
	widget.StateBase[buttonsSection]
	taps int
	last string
}

func (s *buttonsState) bump(what string) {
	s.SetState(func() { s.taps++; s.last = what })
}

func (s *buttonsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Buttons"),
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Default", OnTap: func() { s.bump("Default") }},
			theme.Button{Label: "Primary", Primary: true, OnTap: func() { s.bump("Primary") }},
		}},
		groupLabel("Tappable rows"),
		theme.Body("Row-level press feedback for list items — the highlight flashes on press and eases out on release."),
		widget.Sized{H: 8},
		widget.Decorated{Color: th.Surface, Radius: th.Radius, Child: widget.Column(
			tapRow(th, "Archive", "swipe-free tap target", func() { s.bump("Archive") }),
			divider(th),
			tapRow(th, "Mute", "with a haptic tick", func() { s.bump("Mute") }),
			divider(th),
			tapRow(th, "Delete", "", func() { s.bump("Delete") }),
		)},
		widget.Sized{H: 16},
		theme.Card{Child: widget.Text{
			Value: fmt.Sprintf("Taps: %d       last: %s", s.taps, orDash(s.last)),
			Size:  th.Type.Body,
			Color: th.Text,
		}},
	)
}

func tapRow(th theme.Theme, title, sub string, onTap func()) widget.Widget {
	label := widget.Column(
		widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Body, Color: th.Text},
	)
	label.CrossAlign = layout.CrossStart
	if sub != "" {
		label.Children = append(label.Children,
			widget.Sized{H: 2},
			widget.Text{Value: sub, Size: th.Type.Caption, Color: th.Muted},
		)
	}
	row := widget.Row(widget.Expand(label), widget.Text{Value: "›", Size: th.Type.Title, Color: th.Muted})
	return theme.Tappable{
		Background: th.Surface,
		Pad:        geom.InsetsSymmetric(14, 12),
		Haptic:     true,
		OnTap:      onTap,
		Child:      row,
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- Form controls -----------------------------------------------------------

// formSection binds Switch, Checkbox, a Radio group, and a Slider to live state
// — the biggest gap in the old gallery, so it gets the most attention.
type formSection struct{}

func (formSection) CreateState() widget.State { return &formState{} }

type formState struct {
	widget.StateBase[formSection]
	notify   bool
	toppings [3]bool
	plan     int
	volume   float32
}

func (s *formState) Init(widget.Ctx) {
	s.toppings = [3]bool{true, false, false}
	s.volume = 0.4
}

var toppingNames = []string{"Mushroom", "Olive", "Basil"}
var planNames = []string{"Free", "Pro", "Team"}

func (s *formState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	// Switch: toggles a visible confirmation surface.
	var switchState widget.Widget = theme.Body("Notifications are off.")
	if s.notify {
		switchState = theme.Card{Child: widget.Row(
			widget.Text{Value: "🔔", Size: th.Type.Heading},
			widget.Sized{W: 8},
			widget.Text{Value: "You'll be notified.", Size: th.Type.Body, Color: th.Text},
		)}
	}

	// Checkboxes: three bound toppings, echoed in a summary line.
	checks := make([]widget.Widget, 0, 5)
	for i, name := range toppingNames {
		if i > 0 {
			checks = append(checks, widget.Sized{H: 8})
		}
		checks = append(checks, theme.Checkbox{
			Checked:  s.toppings[i],
			Label:    name,
			OnChange: func(v bool) { s.SetState(func() { s.toppings[i] = v }) },
		})
	}

	// Radio group: single-select plan.
	radios := make([]widget.Widget, 0, 5)
	for i, name := range planNames {
		if i > 0 {
			radios = append(radios, widget.Sized{H: 8})
		}
		radios = append(radios, theme.Radio{
			Selected: s.plan == i,
			Label:    name,
			OnSelect: func() { s.SetState(func() { s.plan = i }) },
		})
	}

	return sectionColumn(
		groupLabel("Switch"),
		widget.Row(
			widget.Expand(theme.Body("Enable notifications")),
			theme.Switch{
				On:       s.notify,
				Label:    "Enable notifications",
				OnChange: func(v bool) { s.SetState(func() { s.notify = v }) },
			},
		),
		widget.Sized{H: 10},
		switchState,

		groupLabel("Checkbox"),
		leftColumn(checks...),
		widget.Sized{H: 8},
		widget.Text{Value: "Chosen: " + orDash(chosen(s.toppings[:], toppingNames)), Size: th.Type.Label, Color: th.Muted},

		groupLabel("Radio group"),
		leftColumn(radios...),
		widget.Sized{H: 8},
		widget.Text{Value: "Plan: " + planNames[s.plan], Size: th.Type.Label, Color: th.Muted},

		groupLabel("Slider"),
		widget.Row(
			widget.Expand(theme.Body("Volume")),
			widget.Text{Value: fmt.Sprintf("%d%%", int(s.volume*100+0.5)), Font: theme.FontBold, Size: th.Type.Body, Color: th.Primary},
		),
		widget.Sized{H: 4},
		theme.Slider{Value: s.volume, OnChange: func(v float32) { s.SetState(func() { s.volume = v }) }},
	)
}

func chosen(flags []bool, names []string) string {
	out := ""
	for i, on := range flags {
		if on {
			if out != "" {
				out += ", "
			}
			out += names[i]
		}
	}
	return out
}

// --- Text input --------------------------------------------------------------

// textInputSection echoes what you type through a themed Field, and keeps a
// multiline note — exercising the caret, selection, and controlled value.
type textInputSection struct{}

func (textInputSection) CreateState() widget.State { return &textInputState{} }

type textInputState struct {
	widget.StateBase[textInputSection]
	name string
	note string
}

func (s *textInputState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	echo := orDash(s.name)
	return sectionColumn(
		groupLabel("Single line"),
		theme.Field{
			Value:       s.name,
			Placeholder: "Your name…",
			OnChange:    func(v string) { s.SetState(func() { s.name = v }) },
		},
		widget.Sized{H: 10},
		widget.Row(
			widget.Text{Value: "Hello, ", Size: th.Type.Body, Color: th.Muted},
			widget.Text{Value: echo, Font: theme.FontBold, Size: th.Type.Body, Color: th.Text},
		),
		widget.Sized{H: 4},
		widget.Text{Value: fmt.Sprintf("%d characters", len([]rune(s.name))), Size: th.Type.Caption, Color: th.Muted},

		groupLabel("Multiline"),
		theme.Field{
			Value:       s.note,
			Placeholder: "A short note (Enter for a new line)…",
			Multiline:   true,
			OnChange:    func(v string) { s.SetState(func() { s.note = v }) },
		},
	)
}

// --- Typography --------------------------------------------------------------

// typographySection is a specimen sheet: each type role at its scale size,
// labeled, so the ramp reads at a glance and re-themes with the switcher.
type typographySection struct{}

func (typographySection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	row := func(name string, px float32, sample widget.Widget) widget.Widget {
		meta := widget.Column(
			widget.Text{Value: name, Font: theme.FontBold, Size: th.Type.Label, Color: th.Text},
			widget.Text{Value: fmt.Sprintf("%gpx", px), Size: th.Type.Caption, Color: th.Muted},
		)
		meta.CrossAlign = layout.CrossStart
		r := widget.Row(widget.Sized{W: 92, Child: meta}, widget.Expand(sample))
		r.CrossAlign = layout.CrossCenter
		return r
	}
	sep := func() widget.Widget {
		return widget.Padding{Insets: geom.Insets{Top: 12, Bottom: 12}, Child: divider(th)}
	}
	return sectionColumn(
		row("Display", th.Type.Display, theme.Display("Considered")),
		sep(),
		row("Title", th.Type.Title, theme.Title("Page title")),
		sep(),
		row("Heading", th.Type.Heading, theme.Heading("Section heading")),
		sep(),
		row("Body", th.Type.Body, theme.Body("Body copy is the default reading size for paragraphs of text.")),
		sep(),
		row("Label", th.Type.Label, theme.Label("Control label")),
		sep(),
		row("Caption", th.Type.Caption, theme.Caption("Fine print · timestamps")),
	)
}

// --- Cards & surfaces --------------------------------------------------------

// cardsSection shows the Card surface, raw Decorated (fill + border), and a
// live Opacity group that fades on tap.
type cardsSection struct{}

func (cardsSection) CreateState() widget.State { return &cardsState{} }

type cardsState struct {
	widget.StateBase[cardsSection]
	faded bool
}

func (s *cardsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	alpha := float32(1)
	if s.faded {
		alpha = 0.25
	}
	// Deliberately a filled, high-contrast panel rather than a plain Card. In a
	// light theme a Card is a white surface on an off-white page — about six
	// levels of contrast at full opacity, and none at all once it is faded to a
	// quarter. The demo then looks broken rather than dimmed, which is exactly
	// how it was reported. Fading has to be visible for a fade to be the point.
	fadeTarget := widget.AnimateFloat(alpha, 200*time.Millisecond, func(a float32) widget.Widget {
		return widget.Opacity{Alpha: a, Child: widget.Decorated{
			Color:  th.Primary,
			Radius: th.Radius,
			Child: widget.Padding{All: 16, Child: widget.Column(
				widget.Text{Value: "Grouped opacity", Font: theme.FontBold, Size: th.Type.Heading, Color: th.OnPrimary},
				widget.Sized{H: 6},
				widget.Text{
					Value: "The whole panel fades as one group, not shape by shape.",
					Size:  th.Type.Body,
					Color: th.OnPrimary,
				},
			)},
		}}
	})

	return sectionColumn(
		groupLabel("Card"),
		theme.Card{Child: widget.Column(
			widget.Text{Value: "A themed surface", Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text},
			widget.Sized{H: 6},
			theme.Body("Card supplies the surface fill, corner radius, and — under a glass theme — the backdrop blur."),
		)},

		groupLabel("Decorated"),
		widget.Row(
			widget.Expand(widget.Decorated{Color: th.Surface, Radius: th.Radius,
				Child: widget.Padding{All: 16, Child: widget.Center(widget.Text{Value: "Filled", Size: th.Type.Body, Color: th.Text})}}),
			widget.Sized{W: 12},
			widget.Expand(widget.Decorated{Radius: th.Radius, BorderColor: th.Border, BorderWidth: 1.5,
				Child: widget.Padding{All: 16, Child: widget.Center(widget.Text{Value: "Bordered", Size: th.Type.Body, Color: th.Text})}}),
		),

		groupLabel("Opacity"),
		widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { s.SetState(func() { s.faded = !s.faded }) }},
			Child:    fadeTarget,
		},
		widget.Sized{H: 8},
		theme.Label("Tap the card to fade it"),
	)
}
