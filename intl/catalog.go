package intl

import (
	"strconv"
	"strings"
	"sync"
)

// Message translation and plural selection.
//
// Formatting (numbers, money, dates) is about how a *place* writes things and
// lives on Locale. Translation is about a *language*, and plural agreement is a
// property of the language's grammar rather than the country's conventions —
// "en-GB" and "en-US" spell dates differently and pluralise identically. So the
// catalog is keyed by language, and a lookup for "pt-BR" falls back to "pt"
// before giving up.
//
// The plural rules here are CLDR's cardinal categories, reduced to the six the
// specification defines. Most languages need two; several need one; Slavic and
// Celtic languages need four or more, and getting those wrong is the difference
// between software that reads as translated and software that reads as
// machine-translated.

// Plural is a CLDR cardinal category.
//
// A language uses a subset: English uses One and Other, Japanese only Other,
// Russian One, Few, Many and Other. A catalog entry supplies the forms its
// language needs, and Select falls back to Other, which every language has.
type Plural uint8

const (
	Other Plural = iota // the required category; the only one in e.g. Japanese
	Zero
	One
	Two
	Few
	Many
)

// String names the category as CLDR does, which is also how catalog authors
// write it.
func (p Plural) String() string {
	switch p {
	case Zero:
		return "zero"
	case One:
		return "one"
	case Two:
		return "two"
	case Few:
		return "few"
	case Many:
		return "many"
	}
	return "other"
}

// ParsePlural reads a CLDR category name.
func ParsePlural(s string) (Plural, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zero":
		return Zero, true
	case "one":
		return One, true
	case "two":
		return Two, true
	case "few":
		return Few, true
	case "many":
		return Many, true
	case "other":
		return Other, true
	}
	return Other, false
}

// Forms holds one message's plural variants. Other is required; the rest are
// filled in only for languages that use them.
type Forms map[Plural]string

// Catalog holds translations for one language.
//
// Safe for concurrent reads once built, which is the usual shape: a program
// loads its catalogs at startup and reads them from the UI goroutine and any
// number of others.
type Catalog struct {
	lang string
	mu   sync.RWMutex
	msgs map[string]Forms
}

// NewCatalog creates an empty catalog for a language tag. Only the language
// subtag is kept: "pt-BR" and "pt" share grammar, and a message looked up under
// either should find the other.
func NewCatalog(tag string) *Catalog {
	return &Catalog{lang: language(tag), msgs: map[string]Forms{}}
}

// Lang returns the catalog's language subtag.
func (c *Catalog) Lang() string { return c.lang }

// Set records a simple message with no plural forms.
func (c *Catalog) Set(key, text string) { c.SetForms(key, Forms{Other: text}) }

// SetForms records a message's plural variants.
func (c *Catalog) SetForms(key string, f Forms) {
	if f == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs[key] = f
}

// Get returns the translation for key, or key itself when absent.
//
// Returning the key rather than an error or an empty string is deliberate: a
// missing translation should leave the UI usable and obviously untranslated,
// not blank. A blank label is a bug report; "checkout.button" is a translation
// task.
func (c *Catalog) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if f, ok := c.msgs[key]; ok {
		if s, ok := f[Other]; ok {
			return s
		}
	}
	return key
}

// Plural returns the translation for key agreeing with n, substituting n for
// any "{n}" in the chosen form.
func (c *Catalog) Plural(key string, n int) string {
	c.mu.RLock()
	f, ok := c.msgs[key]
	c.mu.RUnlock()
	if !ok {
		return key
	}
	cat := PluralFor(c.lang, n)
	s, ok := f[cat]
	if !ok {
		// Every language has Other; a catalog missing the form its own rules
		// selected is an authoring gap, and the required form is the best
		// available answer.
		if s, ok = f[Other]; !ok {
			return key
		}
	}
	return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
}

// language extracts the language subtag from a BCP-47 tag.
func language(tag string) string {
	tag = strings.TrimSpace(tag)
	for i, r := range tag {
		if r == '-' || r == '_' {
			return strings.ToLower(tag[:i])
		}
	}
	return strings.ToLower(tag)
}

// PluralFor reports the CLDR cardinal category for n in a language.
//
// Covers the rule sets that differ from English, which is what actually matters:
// a framework that pluralises every language like English is wrong for most of
// the world's speakers, while one that knows the handful of distinct families
// is right for most of them. Unlisted languages use the English-style
// one/other rule, and languages with no plural distinction return Other.
//
// Only integers are handled. CLDR's rules also key on fraction digits (English
// "1.0 miles" is Other, not One), which matters for formatted quantities and
// not for the counts a UI usually pluralises.
func PluralFor(tag string, n int) Plural {
	lang := language(tag)
	if n < 0 {
		n = -n
	}
	switch lang {
	// No plural distinction at all.
	case "ja", "zh", "ko", "vi", "th", "id", "ms", "my", "km", "lo":
		return Other

	// French and friends: 0 and 1 are singular.
	case "fr", "pt", "hy", "ff", "kab":
		if n == 0 || n == 1 {
			return One
		}
		return Other

	// Russian, Ukrainian, Serbo-Croatian: by last digit, with the teens
	// exception that catches naive implementations.
	case "ru", "uk", "be", "sr", "hr", "bs":
		switch {
		case n%10 == 1 && n%100 != 11:
			return One
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return Few
		default:
			return Many
		}

	// Polish distinguishes Many for the rest, unlike Russian's grouping.
	case "pl":
		switch {
		case n == 1:
			return One
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return Few
		default:
			return Many
		}

	// Czech and Slovak: 2–4 is its own category.
	case "cs", "sk":
		switch {
		case n == 1:
			return One
		case n >= 2 && n <= 4:
			return Few
		default:
			return Other
		}

	// Arabic uses all six.
	case "ar":
		switch {
		case n == 0:
			return Zero
		case n == 1:
			return One
		case n == 2:
			return Two
		case n%100 >= 3 && n%100 <= 10:
			return Few
		case n%100 >= 11 && n%100 <= 99:
			return Many
		default:
			return Other
		}

	// Welsh, also six.
	case "cy":
		switch n {
		case 0:
			return Zero
		case 1:
			return One
		case 2:
			return Two
		case 3:
			return Few
		case 6:
			return Many
		}
		return Other

	// Irish: one, two, few, many, other.
	case "ga":
		switch {
		case n == 1:
			return One
		case n == 2:
			return Two
		case n >= 3 && n <= 6:
			return Few
		case n >= 7 && n <= 10:
			return Many
		default:
			return Other
		}

	// Lithuanian.
	case "lt":
		switch {
		case n%10 == 1 && (n%100 < 11 || n%100 > 19):
			return One
		case n%10 >= 2 && n%10 <= 9 && (n%100 < 11 || n%100 > 19):
			return Few
		default:
			return Other
		}

	// Latvian has a zero category driven by the tens.
	case "lv":
		switch {
		case n%10 == 0 || (n%100 >= 11 && n%100 <= 19):
			return Zero
		case n%10 == 1 && n%100 != 11:
			return One
		default:
			return Other
		}

	// Romanian: few covers 0 and 2–19.
	case "ro":
		switch {
		case n == 1:
			return One
		case n == 0 || (n%100 >= 1 && n%100 <= 19):
			return Few
		default:
			return Other
		}
	}

	// The English-style default, which is also correct for German, Spanish,
	// Italian, Dutch, the Nordics and many more.
	if n == 1 {
		return One
	}
	return Other
}
