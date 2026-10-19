package intl

import (
	"strconv"
	"time"
)

// Relative time formatting: "3 days ago", "in 2 hours", "yesterday".
//
// # Why this is catalog-driven rather than table-driven
//
// golang.org/x/text has no relative-time formatter. golang.org/x/text/date
// carries the CLDR date fields — including the relative ones — but it is five
// megabytes of tables behind an unexported API: the package exports nothing but
// a CLDRVersion constant, so there is nothing to call and nothing to extract
// without vendoring the generator. Checked against v0.41.0, the version in
// go.mod.
//
// That leaves writing the phrasings by hand, and this package will not invent
// locale data from memory: "il y a 3 jours" written from recollection is the
// failure mode the rest of this package is organised against, because a wrong
// translation ships silently and only the people who cannot read around it ever
// see it. So the *decomposition* — which unit, how many, past or future, named
// or numeric — lives here, where it is locale-independent and testable, and the
// *wording* comes from the Catalog, where a translator owns it. English ships
// built in because English is this framework's source language, not because it
// was looked up.
//
// # Message keys
//
// The keys mirror CLDR's own structure so a translator can transcribe
// ldml/dates/fields/field[@type="day"] directly:
//
//	reltime.<unit>.past      plural forms, "{n} days ago"
//	reltime.<unit>.future    plural forms, "in {n} days"
//	reltime.<unit>.<offset>  a single named form, offset in -2..2
//
// where <unit> is one of second, minute, hour, day, week, month, year, and
// <offset> is CLDR's relative[@type] — "reltime.day.-1" is "yesterday".
//
// # The named/numeric boundary
//
// Locales disagree about whether to say "yesterday" or "1 day ago", and they
// disagree about how far the named forms reach: English stops at ±1, while
// several languages have a word for the day before yesterday and none for
// "2 weeks ago". Hard-coding a reach would therefore be wrong somewhere, and
// hard-coding "always count" loses "yesterday" everywhere.
//
// So the reach is data. RelativeNamed asks for a word; the renderer uses one
// only when the catalog actually supplies that offset's key, and otherwise
// counts. A language gets named forms exactly as far as its translator wrote
// them, which is the same place CLDR's own answer lives, and adding
// "reltime.day.-2" to a Hungarian catalog is all it takes to get "tegnapelőtt"
// without touching this package. RelativeNumeric suppresses the lookup
// entirely, for the table column or audit log where "yesterday" is a worse
// answer than a count.
//
// Decompose exposes the decision, so a caller that wants to render it itself —
// or assert on it in a test — never has to parse the rendered string back.

// RelativeUnit is the unit a relative time is expressed in. The names are
// CLDR's field types, which is also how the message keys spell them.
type RelativeUnit uint8

const (
	UnitSecond RelativeUnit = iota
	UnitMinute
	UnitHour
	UnitDay
	UnitWeek
	UnitMonth
	UnitYear
)

// String names the unit as CLDR and the message keys do.
func (u RelativeUnit) String() string {
	switch u {
	case UnitSecond:
		return "second"
	case UnitMinute:
		return "minute"
	case UnitHour:
		return "hour"
	case UnitDay:
		return "day"
	case UnitWeek:
		return "week"
	case UnitMonth:
		return "month"
	}
	return "year"
}

// RelativeStyle selects whether a named form may be used.
type RelativeStyle uint8

const (
	// RelativeNamed prefers a word where the catalog has one — "yesterday",
	// "last month" — and counts where it does not.
	RelativeNamed RelativeStyle = iota
	// RelativeNumeric always counts, which is what a sortable column or an
	// audit trail wants: "1 day ago" beside "2 days ago" reads as a series,
	// "yesterday" beside "2 days ago" does not.
	RelativeNumeric
)

// Relative is a time difference decomposed into a unit and a signed count.
//
// Count is negative for the past, so Unit/Count together are enough to render
// the phrase without re-deriving anything: a renderer picks the "past" or
// "future" key from the sign and the plural category from the magnitude.
type Relative struct {
	Unit  RelativeUnit
	Count int
	// Named reports that a named form was asked for and is available at this
	// offset — that Key returns an offset key rather than a past/future one.
	Named bool
}

// Key returns the catalog key for this decomposition.
func (r Relative) Key() string {
	if r.Named {
		return "reltime." + r.Unit.String() + "." + strconv.Itoa(r.Count)
	}
	if r.Count < 0 {
		return "reltime." + r.Unit.String() + ".past"
	}
	return "reltime." + r.Unit.String() + ".future"
}

// Decompose expresses then relative to now.
//
// # The ladder, and why it is calendar-aware above an hour
//
// Below an hour the clock is the whole story, so seconds and minutes come
// straight from the difference. From an hour up the answer is about calendar
// days, not multiples of 86400 seconds: 23:00 last night is "yesterday" to
// every reader and "2 hours ago" to a duration-only formatter, while 01:00 and
// 23:00 on the same day are 22 hours apart and still "today". Getting that
// wrong is the bug every hand-rolled "time ago" helper has, and it is most
// visible exactly when a timestamp matters most — just after midnight.
//
//	|diff| < 60s                     second
//	|diff| < 60m                     minute
//	same calendar day                hour
//	|days| < 7                       day
//	|days| < 28                      week    (days/7)
//	|months| < 12                    month
//	otherwise                        year    (months/12)
//
// Calendar arithmetic happens in now's location, because that is the reader's
// day. Months are counted as calendar months and then adjusted by day-of-month,
// so 31 January to 1 March is one month rather than two, and a difference that
// adjusts to zero months is reported as one — 28 days is "in 1 month" rather
// than "in 0 months", which is the same choice every UI makes.
//
// Named forms are offered for offsets in -2..2. The caller's catalog decides
// which of those actually exist; see the package note on the named/numeric
// boundary. Note that "today" and "this hour" are unreachable by construction:
// a same-day difference is always reported in hours or finer, where the clock
// says more than the word would.
func Decompose(then, now time.Time, style RelativeStyle) Relative {
	loc := now.Location()
	then, now = then.In(loc), now.In(loc)

	secs := int(then.Sub(now) / time.Second)
	if abs(secs) < 60 {
		return relative(UnitSecond, secs, style)
	}
	if mins := secs / 60; abs(mins) < 60 {
		return relative(UnitMinute, mins, style)
	}

	ty, tm, td := then.Date()
	ny, nm, nd := now.Date()
	thenDay := time.Date(ty, tm, td, 0, 0, 0, 0, loc)
	nowDay := time.Date(ny, nm, nd, 0, 0, 0, 0, loc)
	days := int(thenDay.Sub(nowDay) / (24 * time.Hour))

	switch {
	case days == 0:
		return relative(UnitHour, secs/3600, style)
	case abs(days) < 7:
		return relative(UnitDay, days, style)
	case abs(days) < 28:
		return relative(UnitWeek, days/7, style)
	}

	months := (ty-ny)*12 + int(tm) - int(nm)
	// Day-of-month adjustment: a calendar month boundary crossed with fewer
	// than a month's days elapsed is not a month yet.
	if months > 0 && td < nd {
		months--
	} else if months < 0 && td > nd {
		months++
	}
	if months == 0 {
		// 28 or more days apart but inside one adjusted calendar month. Round
		// away from zero rather than reporting "0 months", which reads as
		// "now" for a difference of a month.
		if days < 0 {
			months = -1
		} else {
			months = 1
		}
	}
	if abs(months) < 12 {
		return relative(UnitMonth, months, style)
	}
	return relative(UnitYear, months/12, style)
}

func relative(u RelativeUnit, count int, style RelativeStyle) Relative {
	// CLDR's relative[@type] values run -2..2; outside that every language
	// counts, so there is nothing to look up.
	named := style == RelativeNamed && count >= -2 && count <= 2
	return Relative{Unit: u, Count: count, Named: named}
}

// RelativeTime renders then relative to now through this catalog.
//
// Lookup order: the named form when one was asked for and the catalog has it,
// then the numeric past/future form, then the built-in English phrasing. The
// English fallback is deliberate and is the one place this package prefers a
// wrong language to a visible key: "reltime.day.past" in a timestamp column is
// unreadable, while "3 days ago" in an otherwise German UI is understood and
// obviously an untranslated string. Catalog.Get returns the key for the
// opposite reason — a label has room to look broken.
func (c *Catalog) RelativeTime(then, now time.Time, style RelativeStyle) string {
	r := Decompose(then, now, style)
	if r.Named {
		if f, ok := c.relForm(r.Key()); ok {
			if s, ok := f[Other]; ok && s != "" {
				return s
			}
		}
		// No word at this offset in this language: count instead. This is the
		// named/numeric boundary, and the catalog just decided it.
		r.Named = false
	}
	key := r.Key()
	f, ok := c.relForm(key)
	if !ok {
		return key
	}
	n := abs(r.Count)
	return renderForm(f, PluralFor(c.lang, n), n, key)
}

// relForm reads a relative-time entry: the catalog first, then the built-in
// English phrasing. A catalog that carries the key at all wins outright, even
// if it is missing the plural form the rules selected — renderForm's Other
// fallback is the right answer there, and silently reaching past a translated
// entry into English would mix two languages inside one phrase.
func (c *Catalog) relForm(key string) (Forms, bool) {
	c.mu.RLock()
	f, ok := c.msgs[key]
	c.mu.RUnlock()
	if ok {
		return f, true
	}
	f, ok = englishRelative[key]
	return f, ok
}

// englishRelative is the built-in fallback phrasing.
//
// English, and only English, because it is this framework's source language:
// writing it out is authorship, not a recollection of CLDR data, and the same
// cannot be said of any other language. Every other language's forms are a
// translator's to supply, under exactly these keys.
//
// The named forms stop where English's do. There is no "reltime.day.-2": the
// language has no single word for the day before yesterday, so a RelativeNamed
// request at that offset falls through to "2 days ago", which is the whole
// mechanism working as intended rather than a gap.
var englishRelative = map[string]Forms{
	"reltime.second.0":      {Other: "now"},
	"reltime.second.past":   {One: "{n} second ago", Other: "{n} seconds ago"},
	"reltime.second.future": {One: "in {n} second", Other: "in {n} seconds"},

	"reltime.minute.past":   {One: "{n} minute ago", Other: "{n} minutes ago"},
	"reltime.minute.future": {One: "in {n} minute", Other: "in {n} minutes"},

	"reltime.hour.past":   {One: "{n} hour ago", Other: "{n} hours ago"},
	"reltime.hour.future": {One: "in {n} hour", Other: "in {n} hours"},

	"reltime.day.-1":     {Other: "yesterday"},
	"reltime.day.0":      {Other: "today"},
	"reltime.day.1":      {Other: "tomorrow"},
	"reltime.day.past":   {One: "{n} day ago", Other: "{n} days ago"},
	"reltime.day.future": {One: "in {n} day", Other: "in {n} days"},

	"reltime.week.-1":     {Other: "last week"},
	"reltime.week.0":      {Other: "this week"},
	"reltime.week.1":      {Other: "next week"},
	"reltime.week.past":   {One: "{n} week ago", Other: "{n} weeks ago"},
	"reltime.week.future": {One: "in {n} week", Other: "in {n} weeks"},

	"reltime.month.-1":     {Other: "last month"},
	"reltime.month.0":      {Other: "this month"},
	"reltime.month.1":      {Other: "next month"},
	"reltime.month.past":   {One: "{n} month ago", Other: "{n} months ago"},
	"reltime.month.future": {One: "in {n} month", Other: "in {n} months"},

	"reltime.year.-1":     {Other: "last year"},
	"reltime.year.0":      {Other: "this year"},
	"reltime.year.1":      {Other: "next year"},
	"reltime.year.past":   {One: "{n} year ago", Other: "{n} years ago"},
	"reltime.year.future": {One: "in {n} year", Other: "in {n} years"},
}

// RelativeKeys lists every message key the relative-time renderer can ask for,
// in a stable order, so extraction tooling can emit a complete stub for a
// language without the extractor having to know this package's key grammar.
//
// It includes the named offsets English does not use (day.-2, week.2 and so on)
// because another language may: a translator who has a word for them needs the
// key present and empty, not absent.
func RelativeKeys() []string {
	units := []RelativeUnit{UnitSecond, UnitMinute, UnitHour, UnitDay, UnitWeek, UnitMonth, UnitYear}
	keys := make([]string, 0, len(units)*7)
	for _, u := range units {
		for _, off := range []int{-2, -1, 0, 1, 2} {
			keys = append(keys, "reltime."+u.String()+"."+strconv.Itoa(off))
		}
		keys = append(keys, "reltime."+u.String()+".past", "reltime."+u.String()+".future")
	}
	return keys
}
