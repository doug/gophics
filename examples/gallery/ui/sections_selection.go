package ui

import (
	"fmt"
	"time"

	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Selection ---------------------------------------------------------------

// selectionSection binds the single-choice controls — Dropdown, Segmented, and
// Tabs — to live state, echoing the current choice (and, for Tabs, swapping the
// panel content) so each one visibly drives something.
type selectionSection struct{}

func (selectionSection) CreateState() widget.State { return &selectionState{} }

type selectionState struct {
	widget.StateBase[selectionSection]
	fruit   int // Dropdown selection; -1 shows the placeholder
	density int // Segmented selection
	tab     int // Tabs selection
}

var (
	fruitNames   = []string{"Apple", "Blueberry", "Clementine", "Date", "Elderberry"}
	densityNames = []string{"Compact", "Cozy", "Roomy"}
	tabNames     = []string{"Overview", "Specs", "Reviews"}
)

func (s *selectionState) Init(widget.Ctx) {
	s.fruit = -1 // start on the placeholder
	s.density = 1
}

func (s *selectionState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	fruitEcho := "—"
	if s.fruit >= 0 && s.fruit < len(fruitNames) {
		fruitEcho = fruitNames[s.fruit]
	}

	// Tabs drive a small piece of swapped content.
	var panel widget.Widget
	switch s.tab {
	case 0:
		panel = theme.Body("Overview — the shape of the thing at a glance.")
	case 1:
		panel = theme.Body("Specs — the numbers and the fine detail.")
	default:
		panel = theme.Body("Reviews — what people had to say about it.")
	}

	return sectionColumn(
		groupLabel("Dropdown"),
		theme.Dropdown{
			Options:     fruitNames,
			Selected:    s.fruit,
			Placeholder: "Pick a fruit…",
			OnChange:    func(i int) { s.SetState(func() { s.fruit = i }) },
		},
		widget.Sized{H: 8},
		widget.Text{Value: "Selected: " + fruitEcho, Size: th.Type.Label, Color: th.Muted},

		groupLabel("Segmented"),
		theme.Segmented{
			Options:  densityNames,
			Selected: s.density,
			OnChange: func(i int) { s.SetState(func() { s.density = i }) },
		},
		widget.Sized{H: 8},
		widget.Text{Value: "Density: " + densityNames[s.density], Size: th.Type.Label, Color: th.Muted},

		groupLabel("Tabs"),
		theme.Tabs{
			Tabs:     tabNames,
			Selected: s.tab,
			OnChange: func(i int) { s.SetState(func() { s.tab = i }) },
		},
		widget.Sized{H: 12},
		theme.Card{Child: panel},
	)
}

// --- Pickers -----------------------------------------------------------------

// pickersSection triggers the date and time picker dialogs and echoes the
// picked value.
type pickersSection struct{}

func (pickersSection) CreateState() widget.State { return &pickersState{} }

type pickersState struct {
	widget.StateBase[pickersSection]
	date      time.Time
	hasDate   bool
	hour, min int
	hasTime   bool
}

func (s *pickersState) Init(widget.Ctx) {
	s.hour, s.min = 9, 30
}

func (s *pickersState) showDate(ctx widget.Ctx) {
	initial := s.date
	if !s.hasDate {
		initial = time.Now()
	}
	theme.ShowDatePicker(ctx, initial, func(t time.Time) {
		s.SetState(func() { s.date, s.hasDate = t, true })
	})
}

func (s *pickersState) showTime(ctx widget.Ctx) {
	theme.ShowTimePicker(ctx, s.hour, s.min, func(hour, min int) {
		s.SetState(func() { s.hour, s.min, s.hasTime = hour, min, true })
	})
}

func (s *pickersState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	dateEcho := "—"
	if s.hasDate {
		dateEcho = s.date.Format("Mon, Jan 2 2006")
	}
	timeEcho := "—"
	if s.hasTime {
		timeEcho = fmt.Sprintf("%02d:%02d", s.hour, s.min)
	}

	return sectionColumn(
		groupLabel("Date"),
		theme.Body("Opens a month calendar in a dialog; pick a day and it echoes below."),
		widget.Sized{H: 10},
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Pick a date", Primary: true, OnTap: func() { s.showDate(ctx) }},
		}},
		widget.Sized{H: 12},
		theme.Card{Child: widget.Text{Value: "Date: " + dateEcho, Size: th.Type.Body, Color: th.Text}},

		groupLabel("Time"),
		theme.Body("Opens an hour/minute stepper in a dialog; every step reports the new time."),
		widget.Sized{H: 10},
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Pick a time", OnTap: func() { s.showTime(ctx) }},
		}},
		widget.Sized{H: 12},
		theme.Card{Child: widget.Text{Value: "Time: " + timeEcho, Size: th.Type.Body, Color: th.Text}},
	)
}
