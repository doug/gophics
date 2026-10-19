package intl

import "testing"

// The cross-check in cldr_test.go proves the rules match CLDR for every count.
// These tests say what the rules *mean*, so a reader can see the shapes without
// running x/text, and so a failure names the language rather than a number.

// English needs four ordinal categories where it needs two cardinal ones, and
// the teens exception runs on n%100 — which is why 111th is not 111st.
func TestEnglishOrdinal(t *testing.T) {
	for n, want := range map[int]Plural{
		1: One, 21: One, 101: One, 1001: One,
		2: Two, 22: Two, 102: Two,
		3: Few, 23: Few, 103: Few,
		4: Other, 11: Other, 12: Other, 13: Other,
		111: Other, 112: Other, 113: Other,
		0: Other, 100: Other,
	} {
		if got := OrdinalFor("en", n); got != want {
			t.Errorf("OrdinalFor(en, %d) = %v, want %v", n, got, want)
		}
	}
}

// Welsh is the stress case: six ordinal categories, and they do *not* agree
// with its six cardinal categories. Copying the cardinal rule would be wrong
// for 3, 4, 5, 6, 7, 8 and 9.
func TestWelshOrdinalDiffersFromCardinal(t *testing.T) {
	ordinals := map[int]Plural{
		0: Zero, 7: Zero, 8: Zero, 9: Zero,
		1: One, 2: Two, 3: Few, 4: Few, 5: Many, 6: Many,
		10: Other, 11: Other,
	}
	for n, want := range ordinals {
		if got := OrdinalFor("cy", n); got != want {
			t.Errorf("OrdinalFor(cy, %d) = %v, want %v", n, got, want)
		}
	}
	// The two rule sets must actually differ, or one of them is a copy.
	differ := 0
	for n := 0; n <= 10; n++ {
		if OrdinalFor("cy", n) != PluralFor("cy", n) {
			differ++
		}
	}
	if differ == 0 {
		t.Error("Welsh ordinal and cardinal rules are identical; one is a copy of the other")
	}
}

// Russian has four cardinal categories and no ordinal rules whatsoever — every
// Russian ordinal is written the same way. A framework that reused the cardinal
// category here would inflect an ordinal four ways in a language that inflects
// it none.
func TestRussianHasNoOrdinalRules(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 5, 11, 21, 101, 111} {
		if got := OrdinalFor("ru", n); got != Other {
			t.Errorf("OrdinalFor(ru, %d) = %v, want Other", n, got)
		}
	}
	if len(OrdinalCategories("ru")) != 1 {
		t.Errorf("OrdinalCategories(ru) = %v, want just [other]", OrdinalCategories("ru"))
	}
	if len(Categories("ru")) != 4 {
		t.Errorf("Categories(ru) = %v, want four cardinal categories", Categories("ru"))
	}
}

// Rule shapes that are neither English's nor "no rules": a two-digit One
// (Swedish), a Few where other languages have a One (Ukrainian), a list rather
// than arithmetic (Italian), and a two-value One (Hungarian).
func TestOrdinalRuleShapes(t *testing.T) {
	for _, c := range []struct {
		lang string
		n    int
		want Plural
	}{
		{"sv", 1, One}, {"sv", 2, One}, {"sv", 3, Other},
		{"sv", 11, Other}, {"sv", 12, Other}, {"sv", 21, One}, {"sv", 22, One},

		{"uk", 3, Few}, {"uk", 13, Other}, {"uk", 23, Few}, {"uk", 1, Other},

		{"be", 2, Few}, {"be", 3, Few}, {"be", 12, Other}, {"be", 13, Other},
		{"be", 22, Few}, {"be", 1, Other},

		{"it", 8, Many}, {"it", 11, Many}, {"it", 80, Many}, {"it", 800, Many},
		{"it", 1, Other}, {"it", 18, Other}, {"it", 108, Other},

		{"hu", 1, One}, {"hu", 5, One}, {"hu", 2, Other}, {"hu", 11, Other},

		{"fr", 1, One}, {"fr", 2, Other}, {"fr", 11, Other},

		// Unlisted languages get Other, not an English-style guess: see the
		// package note in ordinal.go on why this default differs from
		// PluralFor's.
		{"xx", 1, Other}, {"xx", 2, Other},
		{"de", 1, Other}, {"ja", 1, Other},
	} {
		if got := OrdinalFor(c.lang, c.n); got != c.want {
			t.Errorf("OrdinalFor(%s, %d) = %v, want %v", c.lang, c.n, got, c.want)
		}
	}
}

// The payoff: "1st, 2nd, 3rd, 4th" out of a catalog, which is the whole reason
// the categories exist.
func TestOrdinalCatalogRendersEnglishSuffixes(t *testing.T) {
	c := NewCatalog("en")
	c.SetOrdinalForms("place", Forms{
		One:   "{n}st",
		Two:   "{n}nd",
		Few:   "{n}rd",
		Other: "{n}th",
	})
	for n, want := range map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th",
		11: "11th", 12: "12th", 13: "13th",
		21: "21st", 22: "22nd", 23: "23rd", 24: "24th",
		101: "101st", 111: "111th", 112: "112th", 113: "113th",
	} {
		if got := c.Ordinal("place", n); got != want {
			t.Errorf("Ordinal(place, %d) = %q, want %q", n, got, want)
		}
	}
}

// The cardinal and ordinal namespaces are separate, so one key can carry both
// — "3 days" and "3rd day" are the same noun under different agreement.
func TestOrdinalAndCardinalNamespacesCoexist(t *testing.T) {
	c := NewCatalog("en")
	c.SetForms("day", Forms{One: "{n} day", Other: "{n} days"})
	c.SetOrdinalForms("day", Forms{One: "{n}st day", Two: "{n}nd day", Few: "{n}rd day", Other: "{n}th day"})

	if got := c.Plural("day", 3); got != "3 days" {
		t.Errorf("Plural(day, 3) = %q", got)
	}
	if got := c.Ordinal("day", 3); got != "3rd day" {
		t.Errorf("Ordinal(day, 3) = %q", got)
	}
	// A key present in one namespace and not the other must not leak across.
	c2 := NewCatalog("en")
	c2.SetForms("only.cardinal", Forms{Other: "x"})
	if got := c2.Ordinal("only.cardinal", 1); got != "only.cardinal" {
		t.Errorf("Ordinal of a cardinal-only key = %q, want the key", got)
	}
}

// A missing ordinal form falls back to Other, as Plural does: an authoring gap
// should leave the UI readable.
func TestOrdinalFallsBackToOther(t *testing.T) {
	c := NewCatalog("en")
	c.SetOrdinalForms("nth", Forms{Other: "{n}th"})
	if got := c.Ordinal("nth", 1); got != "1th" {
		t.Errorf("missing One form gave %q, want the Other form", got)
	}
	if got := c.Ordinal("absent", 1); got != "absent" {
		t.Errorf("missing key gave %q, want the key", got)
	}
}

// Categories exists so tooling can emit exactly the forms a language needs.
func TestCategoriesPerLanguage(t *testing.T) {
	for _, c := range []struct {
		lang string
		want int
	}{
		{"ja", 1}, {"en", 2}, {"fr", 2}, {"cs", 3}, {"hr", 3},
		{"ru", 4}, {"pl", 4}, {"ar", 6}, {"cy", 6},
	} {
		if got := Categories(c.lang); len(got) != c.want {
			t.Errorf("Categories(%s) = %v (%d), want %d categories", c.lang, got, len(got), c.want)
		}
	}
	// Other is always present, even for a language with no rules at all.
	for _, lang := range []string{"ja", "xx", "ru"} {
		cats := OrdinalCategories(lang)
		last := cats[len(cats)-1]
		if last != Other {
			t.Errorf("OrdinalCategories(%s) does not end in Other: %v", lang, cats)
		}
	}
}
