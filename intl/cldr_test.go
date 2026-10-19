package intl

import (
	"testing"

	xcurrency "golang.org/x/text/currency"
	xplural "golang.org/x/text/feature/plural"
	xlang "golang.org/x/text/language"
)

// These tests make the hand-written plural rules auditable by machine rather
// than by eye.
//
// golang.org/x/text/feature/plural carries CLDR's own generated rule tables for
// both cardinals and ordinals. It is imported *only here*, in a test: linking
// it costs about 412KB of wasm, which a framework whose web payload is its
// loudest complaint cannot spend so that an app can pluralise one label. The
// rules in catalog.go and ordinal.go are therefore written out by hand and
// verified against CLDR on every `go test` — the data is CLDR's, and the binary
// carries none of it.
//
// A failure here means the hand-written rule disagrees with CLDR, which is the
// expensive kind of bug: wrong locale data ships silently and reads as
// machine-translation to the only people who can see it. Fix the rule, not the
// test, unless x/text has been updated to a CLDR release that genuinely changed
// a language's rules — in which case the diff should say so.

// covered lists every language the two rule sets name, so the cross-check
// cannot quietly stop covering one. Languages that reach a rule set's default
// are included deliberately: "de has no ordinal rules" is a claim about CLDR
// that deserves checking too.
var covered = []string{
	// Cardinal rules in catalog.go name these.
	"ja", "zh", "ko", "vi", "th", "id", "ms", "my", "km", "lo",
	"fr", "pt", "hy", "ff", "kab",
	"ru", "uk", "be", "sr", "hr", "bs",
	"pl", "cs", "sk", "ar", "cy", "ga", "lt", "lv", "ro",
	// Languages the formatting table carries that fall to a rule set's default.
	"en", "de", "es", "it", "nl", "sv", "da", "nb", "fi", "hu", "tr",
}

func xForm(f xplural.Form) Plural {
	switch f {
	case xplural.Zero:
		return Zero
	case xplural.One:
		return One
	case xplural.Two:
		return Two
	case xplural.Few:
		return Few
	case xplural.Many:
		return Many
	}
	return Other
}

// The probe range has to clear every modulus the rules test. The widest is
// n%100 nested inside a condition on n%10, so three digits settles it; 1200
// gives a comfortable margin and still runs in milliseconds.
const probeMax = 1200

func checkAgainstCLDR(t *testing.T, name string, rules *xplural.Rules, got func(string, int) Plural) {
	t.Helper()
	for _, lang := range covered {
		tag, err := xlang.Parse(lang)
		if err != nil {
			t.Fatalf("%s: cannot parse language tag: %v", lang, err)
		}
		bad := 0
		for n := 0; n <= probeMax; n++ {
			// i=n, v=w=f=t=0: an integer with no visible decimals, which is
			// what both PluralFor and OrdinalFor accept.
			want := xForm(rules.MatchPlural(tag, n, 0, 0, 0, 0))
			if g := got(lang, n); g != want {
				bad++
				if bad <= 4 {
					t.Errorf("%s(%s, %d) = %v, CLDR says %v", name, lang, n, g, want)
				}
			}
		}
		if bad > 4 {
			t.Errorf("%s %s: %d of %d values disagree with CLDR", name, lang, bad, probeMax+1)
		}
	}
}

func TestCardinalMatchesCLDR(t *testing.T) {
	checkAgainstCLDR(t, "PluralFor", xplural.Cardinal, PluralFor)
}

func TestOrdinalMatchesCLDR(t *testing.T) {
	checkAgainstCLDR(t, "OrdinalFor", xplural.Ordinal, OrdinalFor)
}

// Categories and OrdinalCategories are derived by probing the rules, so this
// checks the derivation against CLDR rather than against a second hand-written
// list — otherwise the two tables could drift together.
func TestCategoriesMatchCLDR(t *testing.T) {
	for _, lang := range covered {
		tag, _ := xlang.Parse(lang)
		for _, c := range []struct {
			name  string
			rules *xplural.Rules
			got   []Plural
		}{
			{"Categories", xplural.Cardinal, Categories(lang)},
			{"OrdinalCategories", xplural.Ordinal, OrdinalCategories(lang)},
		} {
			want := map[Plural]bool{Other: true}
			for n := 0; n <= probeMax; n++ {
				want[xForm(c.rules.MatchPlural(tag, n, 0, 0, 0, 0))] = true
			}
			if len(c.got) != len(want) {
				t.Errorf("%s(%s) = %v, CLDR uses %d categories", c.name, lang, c.got, len(want))
				continue
			}
			for _, p := range c.got {
				if !want[p] {
					t.Errorf("%s(%s) includes %v, which CLDR does not use", c.name, lang, p)
				}
			}
		}
	}
}

// Per-currency fraction digits come from CLDR's currencyData, which x/text
// exposes as the standard rounding scale. Checking every ISO code x/text knows
// — not only the handful the table names — is what makes "the default is 2"
// a verified claim rather than an assumption.
func TestCurrencyDigitsMatchCLDR(t *testing.T) {
	checked := 0
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			for c := 'A'; c <= 'Z'; c++ {
				code := string([]rune{a, b, c})
				u, err := xcurrency.ParseISO(code)
				if err != nil {
					continue
				}
				checked++
				want, _ := xcurrency.Standard.Rounding(u)
				if got := CurrencyDigits(code); got != want {
					t.Errorf("CurrencyDigits(%q) = %d, CLDR says %d", code, got, want)
				}
			}
		}
	}
	if checked < 250 {
		t.Fatalf("only %d ISO codes resolved; the cross-check is measuring nothing", checked)
	}
}
