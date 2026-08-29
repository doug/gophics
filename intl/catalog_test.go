package intl

import (
	"sync"
	"testing"
)

// A missing translation must leave the UI usable and obviously untranslated. A
// blank label is a bug report; the key itself is a translation task.
func TestMissingKeyReturnsKey(t *testing.T) {
	c := NewCatalog("en")
	if got := c.Get("checkout.button"); got != "checkout.button" {
		t.Errorf("Get(missing) = %q, want the key", got)
	}
	if got := c.Plural("cart.items", 3); got != "cart.items" {
		t.Errorf("Plural(missing) = %q, want the key", got)
	}
}

// A catalog is keyed by language, not by locale: pt-BR and pt share grammar, so
// a lookup under either should find the other.
func TestCatalogKeysByLanguage(t *testing.T) {
	c := NewCatalog("pt-BR")
	if c.Lang() != "pt" {
		t.Errorf("Lang() = %q, want pt", c.Lang())
	}
	if NewCatalog("en_GB").Lang() != "en" {
		t.Error("underscore-separated tags should reduce to the language too")
	}
}

func TestPluralSubstitutesCount(t *testing.T) {
	c := NewCatalog("en")
	c.SetForms("items", Forms{One: "{n} item", Other: "{n} items"})
	for n, want := range map[int]string{0: "0 items", 1: "1 item", 2: "2 items"} {
		if got := c.Plural("items", n); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}

// The teens exception is what naive last-digit implementations get wrong: 11 is
// Many in Russian, not One, despite ending in 1.
func TestRussianTeensException(t *testing.T) {
	for n, want := range map[int]Plural{
		1: One, 21: One, 101: One,
		2: Few, 3: Few, 24: Few,
		5: Many, 11: Many, 12: Many, 14: Many, 111: Many,
	} {
		if got := PluralFor("ru", n); got != want {
			t.Errorf("PluralFor(ru, %d) = %v, want %v", n, got, want)
		}
	}
}

// French counts zero as singular, which English does not — the commonest way a
// framework that assumes English gets a Romance language wrong.
func TestFrenchZeroIsSingular(t *testing.T) {
	if got := PluralFor("fr", 0); got != One {
		t.Errorf("PluralFor(fr, 0) = %v, want One", got)
	}
	if got := PluralFor("en", 0); got != Other {
		t.Errorf("PluralFor(en, 0) = %v, want Other", got)
	}
}

// Languages with no plural distinction must always be Other, or every string
// gets an inflection the language does not have.
func TestNoPluralLanguages(t *testing.T) {
	for _, lang := range []string{"ja", "zh", "ko", "vi", "th"} {
		for _, n := range []int{0, 1, 2, 5, 11, 100} {
			if got := PluralFor(lang, n); got != Other {
				t.Errorf("PluralFor(%s, %d) = %v, want Other", lang, n, got)
			}
		}
	}
}

func TestArabicUsesAllSix(t *testing.T) {
	seen := map[Plural]bool{}
	for _, n := range []int{0, 1, 2, 3, 11, 100} {
		seen[PluralFor("ar", n)] = true
	}
	for _, want := range []Plural{Zero, One, Two, Few, Many, Other} {
		if !seen[want] {
			t.Errorf("Arabic never selected %v across representative counts", want)
		}
	}
}

// Polish and Russian differ for the default case, so one must not be
// implemented as the other.
func TestPolishDiffersFromRussian(t *testing.T) {
	if PluralFor("pl", 1) != One || PluralFor("pl", 2) != Few {
		t.Error("Polish 1/2 categories are wrong")
	}
	if PluralFor("cs", 3) != Few || PluralFor("cs", 5) != Other {
		t.Error("Czech treats 2-4 as Few and the rest as Other")
	}
}

// An unlisted language must still behave sensibly rather than returning Zero.
func TestUnknownLanguageUsesEnglishRule(t *testing.T) {
	if got := PluralFor("xx", 1); got != One {
		t.Errorf("unknown language n=1 gave %v, want One", got)
	}
	if got := PluralFor("xx", 7); got != Other {
		t.Errorf("unknown language n=7 gave %v, want Other", got)
	}
}

// A catalog missing the form its own rules selected is an authoring gap, not a
// crash: Other is required in every language and is the best answer available.
func TestFallsBackToOtherForm(t *testing.T) {
	c := NewCatalog("ru")
	c.SetForms("files", Forms{Other: "{n} файлов"})
	if got := c.Plural("files", 1); got != "1 файлов" {
		t.Errorf("missing One form gave %q, want the Other form", got)
	}
}

func TestParsePlural(t *testing.T) {
	for s, want := range map[string]Plural{"one": One, "FEW": Few, " many ": Many, "other": Other} {
		if got, ok := ParsePlural(s); !ok || got != want {
			t.Errorf("ParsePlural(%q) = %v %v, want %v true", s, got, ok, want)
		}
	}
	if _, ok := ParsePlural("plenty"); ok {
		t.Error("ParsePlural accepted a non-category")
	}
}

// Catalogs are read from the UI goroutine and any number of others.
func TestCatalogConcurrentReads(t *testing.T) {
	c := NewCatalog("en")
	c.SetForms("items", Forms{One: "{n} item", Other: "{n} items"})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 200 {
				_ = c.Plural("items", i)
				_ = c.Get("items")
			}
		})
	}
	wg.Wait()
}

// Negative counts should agree like their magnitude rather than falling to a
// default: "-1 item", not "-1 items".
func TestNegativeCountsAgree(t *testing.T) {
	if got := PluralFor("en", -1); got != One {
		t.Errorf("PluralFor(en, -1) = %v, want One", got)
	}
}
