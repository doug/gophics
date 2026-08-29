// Package intl formats numbers, money and dates the way a reader's locale writes
// them: 1,234.56 in the US, 1.234,56 in Germany, 1 234,56 in France,
// 12,34,567.89 in India.
//
// This matters most where numbers *are* the content — a ledger, a dashboard, a
// data table — because showing a German reader "1,234.56" doesn't merely look
// foreign, it reads as one and a bit rather than twelve hundred.
//
// # Why not x/text
//
// golang.org/x/text/message does this properly, from CLDR, and is the right answer
// for an app that needs full localization. It also carries a large data table into
// every binary, which is a poor trade for a framework whose web build is already
// its biggest complaint and whose apps mostly need the punctuation right. This
// package is deliberately small: the separators, grouping and currency placement
// that differ in practice, with no data beyond a compact table. Apps that need
// full CLDR should use x/text directly — nothing here prevents it.
//
// Formatting works on decimal *strings* ("-1234.56"), never float64, so money
// keeps its exact value; callers hand over digits already rounded to the precision
// they want to show.
package intl

import (
	"os"
	"strings"
)

// Negative selects how a negative value is written.
type Negative uint8

const (
	MinusPrefix Negative = iota // -1,234.56
	Parentheses                 // (1,234.56) — accounting style
	MinusSuffix                 // 1,234.56-
)

// Locale describes how one place writes numbers and money.
type Locale struct {
	// Tag is the BCP-47 name, e.g. "de-DE".
	Tag string
	// Decimal separates the fraction; Group separates thousands ("" for none).
	Decimal string
	Group   string
	// GroupSize is the first group's width and GroupRest each subsequent one.
	// They differ in South Asia, where 12,34,567 groups 3 then 2.
	GroupSize int
	GroupRest int
	// Negative selects the sign style.
	Negative Negative
	// CurrencyBefore puts the symbol in front ("$5.00" vs "5,00 €");
	// CurrencySpace inserts a space between symbol and number.
	CurrencyBefore bool
	CurrencySpace  bool
	// DateOrder is the field order for short dates.
	DateOrder DateOrder
	// DateSep separates date fields.
	DateSep string
}

// DateOrder is the field order of a short date.
type DateOrder uint8

const (
	DMY DateOrder = iota // 13/08/2026
	MDY                  // 08/13/2026
	YMD                  // 2026-08-13
)

// Default is US English, used when a locale can't be determined.
var Default = Locale{
	Tag: "en-US", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3,
	CurrencyBefore: true, DateOrder: MDY, DateSep: "/",
}

// nbsp is the non-breaking space used as a group separator in much of Europe, so
// a number never wraps mid-digits.
const nbsp = " "

// locales covers the writing conventions that actually differ. It is a table, not
// a database: a tag that isn't listed falls back to its language, then to Default.
var locales = map[string]Locale{
	"en-US": Default,
	"en-CA": {Tag: "en-CA", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: YMD, DateSep: "-"},
	"en-GB": {Tag: "en-GB", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: DMY, DateSep: "/"},
	"en-AU": {Tag: "en-AU", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: DMY, DateSep: "/"},
	"en-IN": {Tag: "en-IN", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 2, CurrencyBefore: true, DateOrder: DMY, DateSep: "/"},
	"de-DE": {Tag: "de-DE", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"de-CH": {Tag: "de-CH", Decimal: ".", Group: "'", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"fr-FR": {Tag: "fr-FR", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "/"},
	"fr-CA": {Tag: "fr-CA", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: YMD, DateSep: "-"},
	"es-ES": {Tag: "es-ES", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "/"},
	"it-IT": {Tag: "it-IT", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "/"},
	"nl-NL": {Tag: "nl-NL", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, CurrencySpace: true, DateOrder: DMY, DateSep: "-"},
	"pt-BR": {Tag: "pt-BR", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, CurrencySpace: true, DateOrder: DMY, DateSep: "/"},
	"pt-PT": {Tag: "pt-PT", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "/"},
	"pl-PL": {Tag: "pl-PL", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"ru-RU": {Tag: "ru-RU", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"sv-SE": {Tag: "sv-SE", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: YMD, DateSep: "-"},
	"da-DK": {Tag: "da-DK", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"nb-NO": {Tag: "nb-NO", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"fi-FI": {Tag: "fi-FI", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"cs-CZ": {Tag: "cs-CZ", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"hu-HU": {Tag: "hu-HU", Decimal: ",", Group: nbsp, GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: YMD, DateSep: "."},
	"tr-TR": {Tag: "tr-TR", Decimal: ",", Group: ".", GroupSize: 3, GroupRest: 3, CurrencySpace: true, DateOrder: DMY, DateSep: "."},
	"ja-JP": {Tag: "ja-JP", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: YMD, DateSep: "/"},
	"zh-CN": {Tag: "zh-CN", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: YMD, DateSep: "/"},
	"ko-KR": {Tag: "ko-KR", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3, CurrencyBefore: true, DateOrder: YMD, DateSep: ". "},
}

// primaryRegion is the region a bare or unlisted-region tag resolves to.
//
// Without it the fallback below scanned every tag sharing the language and took
// the alphabetically first, which is not a choice so much as an accident of
// sorting: "en" — and every English region the table does not carry, en-DE,
// en-IE, en-NZ, en-ZA — resolved to en-AU, so an American with the region unset
// got day-first dates. Lookup reported ok=true while doing it, telling the
// caller the table knew the place when it had guessed alphabetically.
//
// Found on an iOS simulator whose language was English and region Germany: the
// platform reported en-DE and gophics formatted as Australian.
//
// The regions are CLDR's likely-subtag defaults, which is what other i18n
// stacks resolve a bare language to. Only languages the table carries in more
// than one region need an entry; anything else already resolves through the
// lang-LANG rule or is unambiguous. This does change "pt" from pt-PT to pt-BR —
// the old answer came from the lang-LANG rule rather than from a decision, and
// Brazil is both CLDR's default and the large majority of Portuguese speakers.
var primaryRegion = map[string]string{
	"en": "US",
	"pt": "BR",
	"zh": "CN",
}

// Lookup returns the locale for a BCP-47 tag, falling back to the language's
// primary region ("de" → de-DE, "en" → en-US) and finally to Default. The
// second result reports an exact or language-level match, so a caller can tell
// "I know this place" from "I guessed".
func Lookup(tag string) (Locale, bool) {
	if tag == "" {
		return Default, false
	}
	norm := strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if l, ok := locales[norm]; ok {
		return l, true
	}
	// Try the language subtag against every region we know, preferring an exact
	// language-region pair like de-DE before any other de-*.
	lang := norm
	if i := strings.IndexByte(norm, '-'); i > 0 {
		lang = norm[:i]
	}
	lang = strings.ToLower(lang)
	if r, ok := primaryRegion[lang]; ok {
		if l, ok := locales[lang+"-"+r]; ok {
			return l, true
		}
	}
	if l, ok := locales[lang+"-"+strings.ToUpper(lang)]; ok {
		return l, true
	}
	best, found := Default, false
	for tag, l := range locales {
		if strings.HasPrefix(tag, lang+"-") && (!found || tag < best.Tag) {
			best, found = l, true
		}
	}
	return best, found
}

// Auto determines the locale from the environment (LC_ALL, LC_NUMERIC, LANG),
// which is what a POSIX shell and most desktop sessions set.
//
// A GUI app launched from a desktop that sets none of these gets Default. Reading
// the platform's own setting (NSLocale, GetUserDefaultLocaleName,
// navigator.language) belongs in the shell layer as a capability; until that
// exists, an app that knows better should set its locale explicitly.
func Auto() Locale {
	for _, key := range []string{"LC_ALL", "LC_NUMERIC", "LANG"} {
		v := os.Getenv(key)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		// Strip the encoding and modifier: "de_DE.UTF-8@euro".
		if i := strings.IndexAny(v, ".@"); i > 0 {
			v = v[:i]
		}
		if l, ok := Lookup(v); ok {
			return l
		}
	}
	return Default
}

// Number formats a decimal string ("-1234.56") in this locale.
//
// The input is taken as-is: this re-punctuates, it never rounds. Callers decide
// precision, which keeps money exact and makes the function total — there is no
// input it can silently ruin.
func (l Locale) Number(digits string) string {
	sign, intPart, frac := split(digits)

	var b strings.Builder
	if sign && l.Negative == Parentheses {
		b.WriteByte('(')
	} else if sign && l.Negative == MinusPrefix {
		b.WriteByte('-')
	}
	b.WriteString(l.group(intPart))
	if frac != "" {
		b.WriteString(l.decimal())
		b.WriteString(frac)
	}
	switch {
	case sign && l.Negative == Parentheses:
		b.WriteByte(')')
	case sign && l.Negative == MinusSuffix:
		b.WriteByte('-')
	}
	return b.String()
}

// Money formats a decimal string with a currency symbol or code, placed and
// spaced as the locale writes it.
func (l Locale) Money(digits, currency string) string {
	n := l.Number(digits)
	if currency == "" {
		return n
	}
	sep := ""
	if l.CurrencySpace {
		sep = nbsp
	}
	if l.CurrencyBefore {
		// A leading sign stays outermost: -$5.00, not $-5.00.
		if strings.HasPrefix(n, "-") {
			return "-" + currency + sep + n[1:]
		}
		return currency + sep + n
	}
	return n + nbsp + currency
}

// Date formats year, month and day in the locale's short style.
func (l Locale) Date(year, month, day int) string {
	y, m, d := itoa(year, 4), itoa(month, 2), itoa(day, 2)
	sep := l.DateSep
	if sep == "" {
		sep = "/"
	}
	switch l.DateOrder {
	case MDY:
		return m + sep + d + sep + y
	case YMD:
		return y + sep + m + sep + d
	default:
		return d + sep + m + sep + y
	}
}

// group inserts separators into an integer digit string, honouring the South
// Asian pattern where only the first group is three wide.
func (l Locale) group(s string) string {
	if l.Group == "" || len(s) <= l.first() {
		return s
	}
	first, rest := l.first(), l.rest()

	// Take the low-order group, then peel off the rest right-to-left.
	head, tail := s[:len(s)-first], s[len(s)-first:]
	parts := []string{tail}
	for len(head) > rest {
		parts = append([]string{head[len(head)-rest:]}, parts...)
		head = head[:len(head)-rest]
	}
	if head != "" {
		parts = append([]string{head}, parts...)
	}
	return strings.Join(parts, l.Group)
}

func (l Locale) first() int {
	if l.GroupSize > 0 {
		return l.GroupSize
	}
	return 3
}

func (l Locale) rest() int {
	if l.GroupRest > 0 {
		return l.GroupRest
	}
	return l.first()
}

func (l Locale) decimal() string {
	if l.Decimal == "" {
		return "."
	}
	return l.Decimal
}

// split breaks a decimal string into sign, integer digits and fraction digits,
// tolerating input that already carries grouping or a leading plus.
func split(s string) (neg bool, intPart, frac string) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' {
			return r
		}
		return -1 // drop any separators the caller left in
	}, s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	} else {
		intPart = s
	}
	if intPart == "" {
		intPart = "0"
	}
	return neg, intPart, frac
}

// itoa renders n zero-padded to at least width digits.
func itoa(n, width int) string {
	if n < 0 {
		n = -n
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	for len(s) < width {
		s = "0" + s
	}
	if s == "" {
		s = "0"
	}
	return s
}
