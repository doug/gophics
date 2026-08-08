package theme

import (
	"fmt"
	"strconv"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Metrics of the calendar surface, in logical px. Fixed so the layout is
// predictable (and testable): the grid math below and the day-cell hit
// targets depend only on these.
const (
	dpPad     = 12 // padding inside the picker
	dpHeaderH = 40 // month/year navigation row
	dpGap     = 6  // gap between header and the weekday initials
	dpWeekH   = 24 // weekday-initials row
	dpCellH   = 38 // a day cell in the grid
	dpDisc    = 32 // diameter of the day highlight disc
)

// weekdayInitials labels the seven columns; the week starts on Sunday.
var weekdayInitials = [7]string{"S", "M", "T", "W", "T", "F", "S"}

// DatePicker is a month-calendar day picker: a weekday header, the month's
// days in a grid, prev/next-month arrows and a month/year title. The selected
// day is filled with the accent, today is ringed, and tapping a day fires
// OnPick. It renders its own layout but no surface — wrap it in a Card, or
// present it with ShowDatePicker, for a card/popover look.
type DatePicker struct {
	// Initial is the selected day and the month first shown; its clock time and
	// location are carried onto the date passed to OnPick. Zero means today.
	Initial time.Time
	// Today is the day marked as "today". Zero means time.Now (pass it
	// explicitly to keep rendering deterministic in tests).
	Today  time.Time
	OnPick func(time.Time)
}

func (d DatePicker) CreateState() widget.State { return &datePickerState{} }

type datePickerState struct {
	widget.StateBase[DatePicker]
	year     int        // year of the month currently shown
	month    time.Month // month currently shown
	selected time.Time  // the highlighted day
}

func (s *datePickerState) Init(widget.Ctx) {
	init := s.W().Initial
	if init.IsZero() {
		init = time.Now()
	}
	s.selected = init
	s.year, s.month = init.Year(), init.Month()
}

// shift advances the shown month by delta months.
func (s *datePickerState) shift(delta int) {
	s.SetState(func() { s.year, s.month = rollMonth(s.year, s.month, delta) })
}

// pick selects day-of-month in the shown month and reports it.
func (s *datePickerState) pick(day int) {
	picked := dateOf(s.year, s.month, day, s.W().Initial)
	s.SetState(func() { s.selected = picked })
	if f := s.W().OnPick; f != nil {
		f(picked)
	}
}

func (s *datePickerState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	today := s.W().Today
	if today.IsZero() {
		today = time.Now()
	}

	// Header: prev arrow, month/year title, next arrow.
	title := fmt.Sprintf("%s %d", s.month.String(), s.year)
	header := widget.Sized{H: dpHeaderH, Child: widget.Row(
		arrowButton(th, true, func() { s.shift(-1) }),
		widget.Spacer(),
		widget.Text{S: title, Font: FontBold, Size: th.Type.Heading, Color: th.Text},
		widget.Spacer(),
		arrowButton(th, false, func() { s.shift(1) }),
	)}

	// Weekday initials, one per column.
	weekCells := make([]widget.Widget, 7)
	for i, w := range weekdayInitials {
		weekCells[i] = widget.Sized{H: dpWeekH, Child: widget.Center(
			widget.Text{S: w, Size: th.Type.Caption, Color: th.Muted},
		)}
	}

	// Day grid: leading blanks to the first weekday, then the month's days.
	lead := int(firstWeekday(s.year, s.month))
	n := daysInMonth(s.year, s.month)
	dayCells := make([]widget.Widget, 0, lead+n)
	for i := 0; i < lead; i++ {
		dayCells = append(dayCells, widget.Sized{H: dpCellH})
	}
	for day := 1; day <= n; day++ {
		day := day
		sel := sameYMD(s.selected, s.year, s.month, day)
		isToday := sameYMD(today, s.year, s.month, day)
		dayCells = append(dayCells, s.dayCell(th, day, sel, isToday))
	}

	col := widget.Column(
		header,
		widget.Sized{H: dpGap},
		widget.Grid{Columns: 7, Children: weekCells},
		widget.Grid{Columns: 7, Children: dayCells},
	)
	col.CrossAlign = layout.CrossStretch // let the grids span the full width
	return widget.Padding{All: dpPad, Child: col}
}

// dayCell is one tappable day: an accent-filled disc when selected, an accent
// ring when it is today, plain otherwise.
func (s *datePickerState) dayCell(th Theme, day int, sel, isToday bool) widget.Widget {
	fill, txt, border := paint.Color{}, th.Text, paint.Color{}
	bw := float32(0)
	switch {
	case sel:
		fill, txt = th.Primary, th.OnPrimary
	case isToday:
		border, txt, bw = th.Primary, th.Primary, 1.5
	}
	disc := widget.Sized{W: dpDisc, H: dpDisc, Child: widget.Decorated{
		Color: fill, Radius: dpDisc / 2, BorderColor: border, BorderWidth: bw,
		Child: widget.Center(widget.Text{S: strconv.Itoa(day), Size: th.Type.Label, Color: txt}),
	}}
	return widget.Sized{H: dpCellH, Child: widget.Center(Tappable{
		Child:  disc,
		Radius: dpDisc / 2,
		Haptic: true,
		OnTap:  func() { s.pick(day) },
	})}
}

// arrowButton is a tappable chevron for month navigation.
func arrowButton(th Theme, left bool, onTap func()) widget.Widget {
	return Tappable{
		Radius: 8,
		OnTap:  onTap,
		Child:  widget.Padding{All: 6, Child: chevron(th, left)},
	}
}

// chevron draws a small left/right arrow head.
func chevron(th Theme, left bool) widget.Widget {
	return widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) {
		mid := size.H / 2
		tip, base := float32(7), float32(13)
		if left {
			tip, base = 13, 7
		}
		c.Line(geom.Pt{X: tip, Y: mid - 5}, geom.Pt{X: base, Y: mid}, 2, th.Muted)
		c.Line(geom.Pt{X: base, Y: mid}, geom.Pt{X: tip, Y: mid + 5}, 2, th.Muted)
	}}
}

// ShowDatePicker presents a DatePicker as a dialog. onPick fires with the
// chosen day and the dialog dismisses.
func ShowDatePicker(ctx widget.Ctx, initial time.Time, onPick func(time.Time)) (dismiss func()) {
	var close func()
	close = ShowDialog(ctx, DatePicker{
		Initial: initial,
		Today:   initial,
		OnPick: func(t time.Time) {
			if close != nil {
				close()
			}
			if onPick != nil {
				onPick(t)
			}
		},
	})
	return close
}

// --- calendar math (pure, so it is directly testable) ---

// firstWeekday returns the weekday of the 1st of the month (Sunday = 0).
func firstWeekday(year int, month time.Month) time.Weekday {
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Weekday()
}

// daysInMonth returns the number of days in the month. It reads day 0 of the
// following month, i.e. the last day of this one.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// rollMonth advances year/month by delta months, normalizing across year
// boundaries (delta may be negative).
func rollMonth(year int, month time.Month, delta int) (int, time.Month) {
	t := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, delta, 0)
	return t.Year(), t.Month()
}

// dateOf builds the date for day-of-month in year/month, carrying ref's clock
// time and location so a picked day keeps the caller's time-of-day.
func dateOf(year int, month time.Month, day int, ref time.Time) time.Time {
	return time.Date(year, month, day, ref.Hour(), ref.Minute(), ref.Second(), ref.Nanosecond(), ref.Location())
}

// sameYMD reports whether t falls on the given calendar day.
func sameYMD(t time.Time, year int, month time.Month, day int) bool {
	return !t.IsZero() && t.Year() == year && t.Month() == month && t.Day() == day
}
