package intl

import (
	"strings"
	"testing"
)

// TestCurrencyDigitsMatchCLDR in cldr_test.go checks every ISO code against
// CLDR. These tests say what the digit count is *for*, so a reader sees why the
// table exists rather than only that it is correct.
func TestCurrencyDigits(t *testing.T) {
	for code, want := range map[string]int{
		"USD": 2, "EUR": 2, "GBP": 2,
		"JPY": 0, "KRW": 0, "ISK": 0, "VND": 0, "XOF": 0,
		"KWD": 3, "BHD": 3, "OMR": 3, "TND": 3,
		"CLF": 4,
		// Case and surrounding space are tolerated: the code often arrives
		// from a database column or a URL.
		"jpy": 0, " kwd ": 3,
		// An unknown or future code gets the default rather than nothing.
		"XYZ": 2, "": 2,
	} {
		if got := CurrencyDigits(code); got != want {
			t.Errorf("CurrencyDigits(%q) = %d, want %d", code, got, want)
		}
	}
}

// The headline case: the locale pattern and the currency's digit count are
// independent, and getting one right while assuming the other still reads as
// foreign. All three of these are the same locale.
func TestCurrencyUsesPerCurrencyDigits(t *testing.T) {
	us, _ := Lookup("en-US")
	for _, c := range []struct {
		digits, code, symbol, want string
	}{
		{"1234.56", "USD", "$", "$1,234.56"},
		{"1234.56", "JPY", "¥", "¥1,235"},      // zero digits, and it rounds
		{"1234", "JPY", "¥", "¥1,234"},         // already integral
		{"1234.5", "KWD", "KD", "KD1,234.500"}, // three digits, padded
		{"1234.5678", "KWD", "KD", "KD1,234.568"},
		{"5", "USD", "$", "$5.00"}, // padded to the currency's digits
		// No symbol: the ISO code stands in, upper-cased.
		{"5", "usd", "", "USD5.00"},
	} {
		if got := us.Currency(c.digits, c.code, c.symbol); got != c.want {
			t.Errorf("en-US Currency(%q,%q,%q) = %q, want %q", c.digits, c.code, c.symbol, got, c.want)
		}
	}
}

// The same amounts through locales that place, space and punctuate differently,
// including a right-to-left one.
func TestCurrencyAcrossLocales(t *testing.T) {
	for _, c := range []struct {
		tag, digits, code, symbol, want string
	}{
		{"de-DE", "1234.5", "EUR", "€", "1.234,50" + nbsp + "€"},
		{"fr-FR", "1234.5", "EUR", "€", "1" + nbsp + "234,50" + nbsp + "€"},
		{"de-CH", "1234.5", "CHF", "CHF", "CHF" + nbsp + "1'234.50"},
		{"ja-JP", "1234.6", "JPY", "¥", "¥1,235"},
		{"en-IN", "1234567.5", "INR", "₹", "₹12,34,567.50"},
		{"sv-SE", "1234.5", "SEK", "kr", "1" + nbsp + "234,50" + nbsp + "kr"},
	} {
		l, ok := Lookup(c.tag)
		if !ok {
			t.Fatalf("%s: not in the table", c.tag)
		}
		if got := l.Currency(c.digits, c.code, c.symbol); got != c.want {
			t.Errorf("%s Currency(%q,%q) = %q, want %q", c.tag, c.digits, c.code, got, c.want)
		}
	}
}

// A right-to-left locale.
//
// The digits, separators and symbol are emitted in logical order and the
// bidirectional algorithm lays them out — which is the only correct thing a
// string-level formatter can do. Reordering the characters here to "look"
// right-to-left would produce a string that renders backwards once a real bidi
// pass ran over it, so the assertion is deliberately on logical order. Arabic
// also exercises a plural rule with all six categories, and SAR's two digits
// against KWD's three in the same script.
func TestCurrencyRightToLeft(t *testing.T) {
	ar := Locale{
		Tag: "ar-SA", Decimal: ".", Group: ",", GroupSize: 3, GroupRest: 3,
		CurrencyBefore: false, CurrencySpace: true, DateOrder: DMY, DateSep: "/",
	}
	for _, c := range []struct {
		digits, code, symbol, want string
	}{
		{"1234.5", "SAR", "ر.س", "1,234.50" + nbsp + "ر.س"},
		{"1234.5", "KWD", "د.ك", "1,234.500" + nbsp + "د.ك"},
		{"-1234.5", "SAR", "ر.س", "-1,234.50" + nbsp + "ر.س"},
	} {
		if got := ar.Currency(c.digits, c.code, c.symbol); got != c.want {
			t.Errorf("ar Currency(%q,%q) = %q, want %q", c.digits, c.code, got, c.want)
		}
	}
	// The separator must be the non-breaking space, as it is everywhere else:
	// a number that can wrap away from its currency is unreadable in any
	// script.
	if got := ar.Currency("1234.5", "SAR", "ر.س"); strings.ContainsRune(got, ' ') {
		t.Errorf("%q contains an ordinary space", got)
	}
}

// Negative amounts: the sign stays outermost in minus style, and in accounting
// style the bracket takes its place and wraps the symbol too — "($1,234.56)",
// which is the form every ledger writes and not "$(1,234.56)".
func TestCurrencyNegatives(t *testing.T) {
	us, _ := Lookup("en-US")
	if got := us.Currency("-1234.56", "USD", "$"); got != "-$1,234.56" {
		t.Errorf("minus style = %q, want -$1,234.56", got)
	}

	acct := us
	acct.Negative = Parentheses
	if got := acct.Currency("-1234.56", "USD", "$"); got != "($1,234.56)" {
		t.Errorf("accounting style = %q, want ($1,234.56)", got)
	}
	if got := acct.Currency("1234.56", "USD", "$"); got != "$1,234.56" {
		t.Errorf("positive in accounting style = %q, should be unchanged", got)
	}

	// A suffix locale keeps the symbol after the brackets, because the symbol's
	// place is the locale's and the brackets are the sign's.
	de, _ := Lookup("de-DE")
	de.Negative = Parentheses
	if got := de.Currency("-1234.5", "EUR", "€"); got != "(1.234,50)"+nbsp+"€" {
		t.Errorf("de-DE accounting = %q", got)
	}

	suffixSign := us
	suffixSign.Negative = MinusSuffix
	if got := suffixSign.Currency("-1234.56", "USD", "$"); got != "$1,234.56-" {
		t.Errorf("suffix sign = %q", got)
	}
}

func TestQuantize(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		// Padding.
		{"5", 2, "5.00"},
		{"5.1", 2, "5.10"},
		{"5", 0, "5"},
		{"5", 3, "5.000"},
		// Truncating with half-away-from-zero rounding.
		{"5.004", 2, "5.00"},
		{"5.005", 2, "5.01"},
		{"5.006", 2, "5.01"},
		{"1234.56", 0, "1235"},
		{"1234.49", 0, "1234"},
		{"0.125", 2, "0.13"}, // half away from zero, not half to even
		{"0.135", 2, "0.14"},
		// Carry across the decimal point and through every digit.
		{"9.99", 1, "10.0"},
		{"9.999", 0, "10"},
		{"0.999", 2, "1.00"},
		{"99.95", 1, "100.0"},
		// Negatives round by magnitude and keep their sign.
		{"-0.125", 2, "-0.13"},
		{"-1234.56", 0, "-1235"},
		// A value that rounds to zero loses the sign: "-0.00" reads as a debt
		// of nothing, and in accounting style it would print "(0.00)".
		{"-0.001", 2, "0.00"},
		{"-0.4", 0, "0"},
		// Already-grouped input is tolerated, as Number tolerates it.
		{"1,234.567", 2, "1234.57"},
		// A negative digit count is clamped rather than panicking.
		{"5.5", -1, "6"},
	} {
		if got := Quantize(c.in, c.n); got != c.want {
			t.Errorf("Quantize(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// Quantize does decimal-string arithmetic so that a value a float64 could not
// hold survives. Round-tripping through a float is the mistake this package
// takes strings to avoid, and it would be invisible until someone's ledger was
// out by a cent.
func TestQuantizeBeyondFloat64(t *testing.T) {
	// 20 significant digits: more than float64's 15–17.
	const big = "12345678901234567.895"
	if got := Quantize(big, 2); got != "12345678901234567.90" {
		t.Errorf("Quantize(%q, 2) = %q, want 12345678901234567.90", big, got)
	}
	// A carry that propagates through seventeen nines.
	if got := Quantize("99999999999999999.995", 2); got != "100000000000000000.00" {
		t.Errorf("carry through all nines = %q", got)
	}
}

// Money still takes the fraction as given, which is what a caller that has
// already decided its own precision needs. Currency is the one that quantizes.
func TestMoneyDoesNotQuantize(t *testing.T) {
	us, _ := Lookup("en-US")
	if got := us.Money("5", "$"); got != "$5" {
		t.Errorf("Money(5) = %q; Money must not add the currency's digits", got)
	}
	if got := us.Money("1234.5678", "$"); got != "$1,234.5678" {
		t.Errorf("Money(1234.5678) = %q; Money must not round", got)
	}
}
