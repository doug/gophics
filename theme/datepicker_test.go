package theme

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

// TestCalendarMath pins the grid arithmetic: the weekday the 1st lands on, the
// day count (including a leap February), and month rollover in both directions.
func TestCalendarMath(t *testing.T) {
	// 1 Aug 2026 is a Saturday.
	if got := firstWeekday(2026, time.August); got != time.Saturday {
		t.Fatalf("firstWeekday(Aug 2026) = %v, want Saturday", got)
	}
	// 1 Feb 2026 is a Sunday (the leading-blank count is 0).
	if got := firstWeekday(2026, time.February); got != time.Sunday {
		t.Fatalf("firstWeekday(Feb 2026) = %v, want Sunday", got)
	}
	if got := daysInMonth(2026, time.August); got != 31 {
		t.Fatalf("daysInMonth(Aug 2026) = %d, want 31", got)
	}
	if got := daysInMonth(2024, time.February); got != 29 { // leap year
		t.Fatalf("daysInMonth(Feb 2024) = %d, want 29", got)
	}
	if got := daysInMonth(2026, time.February); got != 28 {
		t.Fatalf("daysInMonth(Feb 2026) = %d, want 28", got)
	}

	// Next from December rolls into the following January.
	if y, m := rollMonth(2026, time.December, +1); y != 2027 || m != time.January {
		t.Fatalf("rollMonth(Dec 2026,+1) = %d %v, want 2027 January", y, m)
	}
	// Prev from January rolls into the previous December.
	if y, m := rollMonth(2026, time.January, -1); y != 2025 || m != time.December {
		t.Fatalf("rollMonth(Jan 2026,-1) = %d %v, want 2025 December", y, m)
	}

	// dateOf carries the reference clock time and location.
	ref := time.Date(2000, time.January, 1, 9, 30, 15, 0, time.UTC)
	d := dateOf(2026, time.August, 15, ref)
	if d.Year() != 2026 || d.Month() != time.August || d.Day() != 15 {
		t.Fatalf("dateOf date = %v, want 2026-08-15", d)
	}
	if d.Hour() != 9 || d.Minute() != 30 {
		t.Fatalf("dateOf clock = %02d:%02d, want 09:30", d.Hour(), d.Minute())
	}
}

// TestDatePickerPick renders a DatePicker and taps a specific day, asserting
// OnPick fires with that date. It computes the tap point from the fixed
// calendar metrics, so it exercises the real grid layout and hit target.
func TestDatePickerPick(t *testing.T) {
	initial := time.Date(2026, time.August, 1, 8, 45, 0, 0, time.UTC)
	var picked time.Time
	pick := DatePicker{
		Initial: initial,
		Today:   initial,
		OnPick:  func(d time.Time) { picked = d },
	}

	const winW, winH = 320, 420
	h, err := app.NewHeadless(pick, app.Config{
		Size:         geom.Size{W: winW, H: winH},
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: winW, H: winH})

	// Tap the 15th. Grid cells fill the padded width in 7 equal columns.
	const day = 15
	lead := int(firstWeekday(2026, time.August)) // Saturday = 6
	idx := lead + day - 1
	row, coln := idx/7, idx%7
	cellW := float32(winW-2*dpPad) / 7
	x := dpPad + float32(coln)*cellW + cellW/2
	y := dpPad + dpHeaderH + dpGap + dpWeekH + float32(row)*dpCellH + dpCellH/2

	h.Tap(geom.Pt{X: x, Y: y})

	if picked.IsZero() {
		t.Fatalf("tapping day %d did not fire OnPick", day)
	}
	if picked.Year() != 2026 || picked.Month() != time.August || picked.Day() != day {
		t.Fatalf("OnPick date = %v, want 2026-08-%02d", picked, day)
	}
	// The reference clock time is carried through.
	if picked.Hour() != 8 || picked.Minute() != 45 {
		t.Fatalf("OnPick clock = %02d:%02d, want 08:45", picked.Hour(), picked.Minute())
	}
}
