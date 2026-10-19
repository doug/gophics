package intl

import (
	"testing"
	"time"
)

// now is a fixed instant with room on either side: mid-month, mid-year, not
// near a DST transition, so a failure is about the ladder and not the calendar.
var now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func TestDecomposeLadder(t *testing.T) {
	for _, c := range []struct {
		name  string
		then  time.Time
		unit  RelativeUnit
		count int
	}{
		{"same instant", now, UnitSecond, 0},
		{"30s ago", now.Add(-30 * time.Second), UnitSecond, -30},
		{"59s ahead", now.Add(59 * time.Second), UnitSecond, 59},
		{"90s ago", now.Add(-90 * time.Second), UnitMinute, -1},
		{"45m ahead", now.Add(45 * time.Minute), UnitMinute, 45},
		{"59m ago", now.Add(-59 * time.Minute), UnitMinute, -59},

		// Past an hour the answer is about the calendar day.
		{"3h ago, same day", now.Add(-3 * time.Hour), UnitHour, -3},
		{"11h ahead, same day", now.Add(11 * time.Hour), UnitHour, 11},

		{"1 day ago", now.AddDate(0, 0, -1), UnitDay, -1},
		{"6 days ahead", now.AddDate(0, 0, 6), UnitDay, 6},

		{"7 days ago", now.AddDate(0, 0, -7), UnitWeek, -1},
		{"20 days ahead", now.AddDate(0, 0, 20), UnitWeek, 2},
		{"27 days ago", now.AddDate(0, 0, -27), UnitWeek, -3},

		{"2 months ahead", now.AddDate(0, 2, 0), UnitMonth, 2},
		{"5 months ago", now.AddDate(0, -5, 0), UnitMonth, -5},
		{"11 months ahead", now.AddDate(0, 11, 0), UnitMonth, 11},

		{"1 year ahead", now.AddDate(1, 0, 0), UnitYear, 1},
		{"3 years ago", now.AddDate(-3, 0, 0), UnitYear, -3},
	} {
		got := Decompose(c.then, now, RelativeNumeric)
		if got.Unit != c.unit || got.Count != c.count {
			t.Errorf("%s: Decompose = %v %d, want %v %d", c.name, got.Unit, got.Count, c.unit, c.count)
		}
	}
}

// The calendar-aware boundary is the thing every hand-rolled "time ago" helper
// gets wrong, and it is most visible just after midnight: 23:00 last night is
// two hours ago and also yesterday, and the reader means yesterday.
func TestDayBoundaryIsCalendarNotDuration(t *testing.T) {
	justAfterMidnight := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)

	lastNight := time.Date(2026, 6, 14, 23, 0, 0, 0, time.UTC)
	if got := Decompose(lastNight, justAfterMidnight, RelativeNumeric); got.Unit != UnitDay || got.Count != -1 {
		t.Errorf("23:00 yesterday from 01:00 today = %v %d, want day -1", got.Unit, got.Count)
	}

	// The converse: 22 hours apart inside one calendar day stays in hours.
	lateTonight := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	if got := Decompose(lateTonight, justAfterMidnight, RelativeNumeric); got.Unit != UnitHour {
		t.Errorf("22 hours later on the same day = %v, want hours", got.Unit)
	}

	// Under an hour wins over the calendar: 23:50 to 00:10 is twenty minutes,
	// not yesterday.
	tenPast := time.Date(2026, 6, 15, 0, 10, 0, 0, time.UTC)
	tenToMidnight := time.Date(2026, 6, 14, 23, 50, 0, 0, time.UTC)
	if got := Decompose(tenToMidnight, tenPast, RelativeNumeric); got.Unit != UnitMinute || got.Count != -20 {
		t.Errorf("23:50 to 00:10 = %v %d, want minute -20", got.Unit, got.Count)
	}
}

// Calendar arithmetic happens in the reader's zone, so the same instant can be
// "yesterday" in one place and "2 hours ago" in another. That is correct, and
// using the stored timestamp's zone instead would make it wrong for whoever is
// reading.
func TestCalendarUsesTheReadersZone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("capability: this build has no zoneinfo, so the zone-dependent branch cannot be exercised")
	}
	instant := time.Date(2026, 6, 14, 23, 0, 0, 0, time.UTC)
	reader := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)

	if got := Decompose(instant, reader, RelativeNumeric); got.Unit != UnitDay {
		t.Errorf("in UTC the instant is yesterday, got %v", got.Unit)
	}
	// In Tokyo both are on 15 June (08:00 and 10:00), so it is two hours.
	if got := Decompose(instant, reader.In(tokyo), RelativeNumeric); got.Unit != UnitHour {
		t.Errorf("in Asia/Tokyo both fall on the same day; got %v, want hours", got.Unit)
	}
}

// A month boundary crossed with fewer than a month's days elapsed is not a
// month yet, and a difference that adjusts to zero months is still reported as
// one — "in 0 months" would read as "now".
func TestMonthArithmeticAdjustsForDayOfMonth(t *testing.T) {
	jan31 := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	// 1 March is one calendar-month step past 31 January by day-of-month.
	mar1 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if got := Decompose(mar1, jan31, RelativeNumeric); got.Unit != UnitMonth || got.Count != 1 {
		t.Errorf("31 Jan to 1 Mar = %v %d, want month 1", got.Unit, got.Count)
	}

	// 28 days apart, inside one adjusted calendar month: rounded away from zero.
	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	jan29 := time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC)
	if got := Decompose(jan29, jan1, RelativeNumeric); got.Unit != UnitMonth || got.Count != 1 {
		t.Errorf("1 Jan to 29 Jan = %v %d, want month 1 rather than 0", got.Unit, got.Count)
	}
	if got := Decompose(jan1, jan29, RelativeNumeric); got.Unit != UnitMonth || got.Count != -1 {
		t.Errorf("29 Jan back to 1 Jan = %v %d, want month -1", got.Unit, got.Count)
	}
}

func TestRenderEnglishAcrossUnits(t *testing.T) {
	c := NewCatalog("en")
	for _, k := range []struct {
		then time.Time
		want string
	}{
		{now, "now"},
		{now.Add(-1 * time.Second), "1 second ago"},
		{now.Add(-30 * time.Second), "30 seconds ago"},
		{now.Add(90 * time.Second), "in 1 minute"},
		{now.Add(-5 * time.Hour), "5 hours ago"},
		{now.AddDate(0, 0, -1), "yesterday"},
		{now.AddDate(0, 0, 1), "tomorrow"},
		{now.AddDate(0, 0, -3), "3 days ago"},
		{now.AddDate(0, 0, 7), "next week"},
		{now.AddDate(0, 0, -21), "3 weeks ago"},
		{now.AddDate(0, 2, 0), "in 2 months"},
		{now.AddDate(-1, 0, 0), "last year"},
		{now.AddDate(-4, 0, 0), "4 years ago"},
	} {
		if got := c.RelativeTime(k.then, now, RelativeNamed); got != k.want {
			t.Errorf("RelativeTime(%v) = %q, want %q", k.then, got, k.want)
		}
	}
}

// The named/numeric choice is visible to users, so both halves of it are
// asserted: RelativeNumeric never says "yesterday", and RelativeNamed does.
func TestNamedVersusNumeric(t *testing.T) {
	c := NewCatalog("en")
	yesterday := now.AddDate(0, 0, -1)

	if got := c.RelativeTime(yesterday, now, RelativeNamed); got != "yesterday" {
		t.Errorf("RelativeNamed = %q, want yesterday", got)
	}
	if got := c.RelativeTime(yesterday, now, RelativeNumeric); got != "1 day ago" {
		t.Errorf("RelativeNumeric = %q, want 1 day ago", got)
	}
	// A sortable column wants a series, which is the reason the style exists.
	for i, want := range []string{"1 day ago", "2 days ago", "3 days ago"} {
		got := c.RelativeTime(now.AddDate(0, 0, -(i+1)), now, RelativeNumeric)
		if got != want {
			t.Errorf("numeric series[%d] = %q, want %q", i, got, want)
		}
	}
}

// English has no word for the day before yesterday, so a RelativeNamed request
// at that offset falls through to the count. A language whose translator adds
// the key gets the word without this package changing.
func TestNamedFallsBackWhenTheCatalogHasNoWord(t *testing.T) {
	en := NewCatalog("en")
	twoDaysAgo := now.AddDate(0, 0, -2)
	if got := en.RelativeTime(twoDaysAgo, now, RelativeNamed); got != "2 days ago" {
		t.Errorf("en at -2 days = %q, want the numeric form", got)
	}

	// Hungarian has "tegnapelőtt". Adding the key is all it takes.
	hu := NewCatalog("hu")
	hu.SetForms("reltime.day.-2", Forms{Other: "tegnapelőtt"})
	hu.SetForms("reltime.day.past", Forms{Other: "{n} nappal ezelőtt"})
	if got := hu.RelativeTime(twoDaysAgo, now, RelativeNamed); got != "tegnapelőtt" {
		t.Errorf("hu at -2 days = %q, want the named form the catalog supplies", got)
	}
	if got := hu.RelativeTime(twoDaysAgo, now, RelativeNumeric); got != "2 nappal ezelőtt" {
		t.Errorf("hu numeric = %q", got)
	}
	// Decompose reports the offer, not the outcome: the catalog decides.
	if r := Decompose(twoDaysAgo, now, RelativeNamed); !r.Named || r.Key() != "reltime.day.-2" {
		t.Errorf("Decompose at -2 days = %+v, key %q", r, r.Key())
	}
	if r := Decompose(twoDaysAgo, now, RelativeNumeric); r.Named || r.Key() != "reltime.day.past" {
		t.Errorf("numeric style still offered a named form: %+v", r)
	}
}

// A translated catalog overrides the built-in English entirely, and the count
// agrees by the catalog's own language. Russian is the test because its three
// non-default categories make a wrong selection obvious.
func TestTranslatedCatalogUsesItsOwnPluralRules(t *testing.T) {
	ru := NewCatalog("ru")
	ru.SetForms("reltime.minute.past", Forms{
		One:   "{n} минуту назад",
		Few:   "{n} минуты назад",
		Many:  "{n} минут назад",
		Other: "{n} минуты назад",
	})
	// Minutes, because the unit holds counts from 1 to 59 and so reaches all
	// four of Russian's categories — including the teens exception at 11 and
	// the last-digit One at 21, which is where a wrong rule shows.
	for mins, want := range map[int]string{
		3:  "3 минуты назад",
		5:  "5 минут назад",
		11: "11 минут назад",
		21: "21 минуту назад",
	} {
		got := ru.RelativeTime(now.Add(-time.Duration(mins)*time.Minute), now, RelativeNumeric)
		if got != want {
			t.Errorf("ru %d minutes ago = %q, want %q", mins, got, want)
		}
	}
}

// The count is rendered as a magnitude: "ago" already carries the direction and
// "-3 days ago" would state it twice.
func TestCountIsRenderedUnsigned(t *testing.T) {
	c := NewCatalog("en")
	if got := c.RelativeTime(now.AddDate(0, 0, -3), now, RelativeNumeric); got != "3 days ago" {
		t.Errorf("past rendered %q; the sign should not reach the text", got)
	}
}

// RelativeKeys is what extraction emits, so it must cover every key the
// renderer can ask for — including offsets English does not use.
func TestRelativeKeysCoverEveryReachableKey(t *testing.T) {
	have := map[string]bool{}
	for _, k := range RelativeKeys() {
		if have[k] {
			t.Errorf("RelativeKeys repeats %q", k)
		}
		have[k] = true
	}
	// Every built-in English key must be in the set, or a translator handed the
	// extracted stub would be unable to override a string the app can show.
	for k := range englishRelative {
		if !have[k] {
			t.Errorf("RelativeKeys omits %q, which the renderer can look up", k)
		}
	}
	// And every key Decompose can produce, swept across the ladder.
	for _, d := range []time.Duration{
		0, time.Second, -time.Second, time.Minute * 2, -time.Minute * 2,
		time.Hour * 5, -time.Hour * 5,
	} {
		for _, style := range []RelativeStyle{RelativeNamed, RelativeNumeric} {
			if k := Decompose(now.Add(d), now, style).Key(); !have[k] {
				t.Errorf("RelativeKeys omits %q", k)
			}
		}
	}
	for _, days := range []int{-400, -40, -20, -3, -1, 1, 3, 20, 40, 400} {
		for _, style := range []RelativeStyle{RelativeNamed, RelativeNumeric} {
			if k := Decompose(now.AddDate(0, 0, days), now, style).Key(); !have[k] {
				t.Errorf("RelativeKeys omits %q (%d days)", k, days)
			}
		}
	}
}

func TestRelativeUnitNames(t *testing.T) {
	for u, want := range map[RelativeUnit]string{
		UnitSecond: "second", UnitMinute: "minute", UnitHour: "hour",
		UnitDay: "day", UnitWeek: "week", UnitMonth: "month", UnitYear: "year",
	} {
		if got := u.String(); got != want {
			t.Errorf("RelativeUnit(%d).String() = %q, want %q", u, got, want)
		}
	}
}
