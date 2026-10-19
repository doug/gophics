package intl

import (
	"strconv"
	"strings"
)

// Ordinal plural selection: "1st, 2nd, 3rd, 4th" and the equivalents elsewhere.
//
// CLDR keeps two independent rule sets per language, and they are genuinely
// different shapes rather than variations on one another. English needs four
// categories for ordinals (one/two/few/other, by last digit with a teens
// exception) and two for cardinals. German needs four for neither — every
// German ordinal is "N." and the language has no ordinal rules at all. Italian
// has no cardinal surprises and an ordinal rule that fires on exactly four
// numbers (8, 11, 80, 800: "l'ottavo", "l'undicesimo"). So ordinals cannot be
// derived from cardinals, and a framework that reuses the cardinal category for
// an ordinal message is wrong for English — the commonest case — from n=2 on.
//
// # Where the default differs from cardinals
//
// PluralFor falls back to the English one/other rule for a language it does not
// carry, because that rule is right for most of the languages it would meet.
// OrdinalFor falls back to Other, because the opposite is true: of the 41
// languages below, 26 have no ordinal rules at all, and inventing a
// singular/plural split for an unlisted language would inflect an ordinal in a
// language that does not inflect ordinals. Other is the one category every
// language has, so the fallback is "no agreement", not "English agreement".
//
// # Verifying this table
//
// Every rule here is checked against golang.org/x/text/feature/plural's
// Ordinal rules — CLDR's own data, generated from the repository — by
// TestOrdinalMatchesCLDR in cldr_test.go, for every language listed and every
// n in 0..1200. x/text is a test-only import: its tables cost ~412KB of wasm,
// which a framework whose web payload is its biggest complaint cannot spend on
// a feature most apps use for one message. The data is therefore CLDR's and the
// binary carries none of it.
//
// A reader who wants to check by hand reads
// https://unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html,
// "Ordinal" column, or common/supplemental/ordinals.xml in the CLDR release.

// OrdinalFor reports the CLDR ordinal category for n in a language.
//
// Pair it with a catalog entry whose forms carry the suffixes:
//
//	c.SetOrdinalForms("place", intl.Forms{
//		intl.One:   "{n}st",
//		intl.Two:   "{n}nd",
//		intl.Few:   "{n}rd",
//		intl.Other: "{n}th",
//	})
//	c.Ordinal("place", 23) // "23rd"
//
// Only integers are handled, which is all an ordinal is: there is no 2.5th.
func OrdinalFor(tag string, n int) Plural {
	lang := language(tag)
	if n < 0 {
		n = -n
	}
	switch lang {
	// English: by last digit, with the teens exception. 21st, 22nd, 23rd, but
	// 11th, 12th, 13th — and 111th, because the test is on n%100.
	case "en":
		switch {
		case n%10 == 1 && n%100 != 11:
			return One
		case n%10 == 2 && n%100 != 12:
			return Two
		case n%10 == 3 && n%100 != 13:
			return Few
		}
		return Other

	// Swedish: one covers both 1 and 2 as last digits ("1:a", "2:a" against
	// "3:e"), with the same teens exception.
	case "sv":
		if m := n % 10; (m == 1 || m == 2) && n%100 != 11 && n%100 != 12 {
			return One
		}
		return Other

	// Ukrainian uses a single non-default category, and it is Few rather than
	// One — a reminder that the category names are labels on a language's own
	// agreement classes, not a ranking.
	case "uk":
		if n%10 == 3 && n%100 != 13 {
			return Few
		}
		return Other

	// Belarusian: Few on last digits 2 and 3, teens excepted.
	case "be":
		if m := n % 10; (m == 2 || m == 3) && n%100 != 12 && n%100 != 13 {
			return Few
		}
		return Other

	// Italian: Many on exactly four values. The rule is about the initial vowel
	// of the written ordinal, so it is a list and not an arithmetic condition.
	case "it":
		switch n {
		case 8, 11, 80, 800:
			return Many
		}
		return Other

	// Welsh needs all six for ordinals as well as for cardinals, and the two
	// rule sets do not agree: 0 is Zero in both, but 7, 8 and 9 are Zero only
	// as ordinals, and 3 is Few as a cardinal and Few as an ordinal alongside
	// 4. Copying the cardinal rule here would be wrong for eight of the first
	// ten numbers.
	case "cy":
		switch n {
		case 0, 7, 8, 9:
			return Zero
		case 1:
			return One
		case 2:
			return Two
		case 3, 4:
			return Few
		case 5, 6:
			return Many
		}
		return Other

	// Hungarian: 1 and 5 take a different suffix vowel from the rest
	// ("első", "ötödik" against "harmadik"), and nothing else does.
	case "hu":
		if n == 1 || n == 5 {
			return One
		}
		return Other

	// A single One for n=1 and Other for everything else.
	case "fr", "hy", "ga", "lo", "ms", "ro", "vi":
		if n == 1 {
			return One
		}
		return Other
	}

	// No ordinal agreement: de, es, nl, pt, pl, ru, da, nb, fi, cs, sk, tr, ja,
	// zh, ko, th, id, my, km, ff, kab, sr, hr, bs, ar, lt, lv — and any language
	// this table does not carry. See the package note above on why this default
	// differs from PluralFor's.
	return Other
}

// Categories reports which cardinal categories a language actually uses, in
// CLDR's canonical order, so tooling can emit exactly the forms a translator
// has to fill in.
//
// Derived by running the rules rather than by a second table, which is the
// point: a change to PluralFor cannot leave this list stale, and there is no
// way for the two to disagree. The probe range covers every modulus the rules
// test (the largest is n%100 inside a hundreds-sensitive condition), so a
// category that only fires above it does not exist.
func Categories(tag string) []Plural {
	return categories(func(n int) Plural { return PluralFor(tag, n) })
}

// OrdinalCategories reports which ordinal categories a language uses, as
// Categories does for cardinals.
func OrdinalCategories(tag string) []Plural {
	return categories(func(n int) Plural { return OrdinalFor(tag, n) })
}

func categories(sel func(int) Plural) []Plural {
	seen := map[Plural]bool{Other: true} // every language has Other
	for n := 0; n <= 1200; n++ {
		seen[sel(n)] = true
	}
	// CLDR's order, which is also the order catalog files and translation tools
	// present the forms in.
	var out []Plural
	for _, p := range []Plural{Zero, One, Two, Few, Many, Other} {
		if seen[p] {
			out = append(out, p)
		}
	}
	return out
}

// SetOrdinalForms records a message's ordinal variants.
//
// Kept in a separate namespace from SetForms because a key can legitimately
// need both: a "day" message may read "3 days" when counting and "3rd day" when
// ranking, and the two agree by different rules. Collapsing them into one map
// would force translators to invent a second key for the same noun.
func (c *Catalog) SetOrdinalForms(key string, f Forms) {
	if f == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ords == nil {
		c.ords = map[string]Forms{}
	}
	c.ords[key] = f
}

// Ordinal returns the translation for key agreeing with n as an ordinal,
// substituting n for any "{n}" in the chosen form.
//
// Missing keys return the key, and a missing form falls back to Other, for the
// same reasons Plural does: an obviously untranslated UI is a translation task
// and a blank one is a bug report.
func (c *Catalog) Ordinal(key string, n int) string {
	c.mu.RLock()
	f, ok := c.ords[key]
	c.mu.RUnlock()
	if !ok {
		return key
	}
	return renderForm(f, OrdinalFor(c.lang, n), n, key)
}

// renderForm picks the form for a category and substitutes the count. Shared by
// Plural, Ordinal and the relative-time renderer so the fallback rule — the
// selected form, else Other, else the key — is written once.
//
// The sign is substituted as given: a count of -1 renders "-1 item", because a
// caller that hands over a negative quantity means it. The relative-time
// renderer passes a magnitude for that reason, since "ago" already carries the
// direction and "-3 days ago" would say it twice.
func renderForm(f Forms, cat Plural, n int, key string) string {
	s, ok := f[cat]
	if !ok {
		if s, ok = f[Other]; !ok {
			return key
		}
	}
	return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
