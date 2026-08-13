package intl

import (
	"strings"
	"testing"
)

// TestNumberAcrossLocales is the whole point: the same value, written the way each
// place writes it. Showing a German reader "1,234.56" doesn't look foreign, it
// reads as one-and-a-bit.
func TestNumberAcrossLocales(t *testing.T) {
	const v = "1234567.89"
	cases := map[string]string{
		"en-US": "1,234,567.89",
		"en-GB": "1,234,567.89",
		"de-DE": "1.234.567,89",
		"de-CH": "1'234'567.89",
		"fr-FR": "1" + nbsp + "234" + nbsp + "567,89",
		"es-ES": "1.234.567,89",
		"ru-RU": "1" + nbsp + "234" + nbsp + "567,89",
		"sv-SE": "1" + nbsp + "234" + nbsp + "567,89",
		"ja-JP": "1,234,567.89",
		"en-IN": "12,34,567.89", // South Asian grouping: 3 then 2
	}
	for tag, want := range cases {
		l, ok := Lookup(tag)
		if !ok {
			t.Errorf("%s: not found", tag)
			continue
		}
		if got := l.Number(v); got != want {
			t.Errorf("%s: %q, want %q", tag, got, want)
		}
	}
}

func TestNumberEdgeCases(t *testing.T) {
	l := Default
	cases := map[string]string{
		"0":          "0",
		"0.00":       "0.00",
		"1":          "1",
		"12":         "12",
		"123":        "123",
		"1234":       "1,234",
		"-1234.5":    "-1,234.5",
		"+1234":      "1,234",
		".5":         "0.5",
		"1000000":    "1,000,000",
		"-0.01":      "-0.01",
		"1,234.56":   "1,234.56", // already grouped input is re-punctuated, not doubled
		"12345678.9": "12,345,678.9",
	}
	for in, want := range cases {
		if got := l.Number(in); got != want {
			t.Errorf("Number(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNumberPreservesPrecision guards the reason this takes strings: money must
// not be rounded or re-derived by the formatter.
func TestNumberPreservesPrecision(t *testing.T) {
	for _, in := range []string{
		"0.10", "1.005", "123456789012345678.99", "0.000001",
	} {
		got := Default.Number(in)
		stripped := ""
		for _, r := range got {
			if r >= '0' && r <= '9' || r == '.' {
				stripped += string(r)
			}
		}
		want := in
		if want[0] == '+' {
			want = want[1:]
		}
		if stripped != want {
			t.Errorf("Number(%q) = %q; digits became %q", in, got, stripped)
		}
	}
}

func TestIndianGrouping(t *testing.T) {
	l, _ := Lookup("en-IN")
	cases := map[string]string{
		"1":          "1",
		"100":        "100",
		"1000":       "1,000",
		"100000":     "1,00,000",
		"1000000":    "10,00,000",
		"12345678.9": "1,23,45,678.9",
	}
	for in, want := range cases {
		if got := l.Number(in); got != want {
			t.Errorf("en-IN Number(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMoneyPlacement(t *testing.T) {
	cases := []struct {
		tag, digits, cur, want string
	}{
		{"en-US", "1234.56", "$", "$1,234.56"},
		{"en-US", "-1234.56", "$", "-$1,234.56"}, // sign stays outermost
		{"de-DE", "1234.56", "€", "1.234,56" + nbsp + "€"},
		{"fr-FR", "1234.56", "€", "1" + nbsp + "234,56" + nbsp + "€"},
		{"ja-JP", "1234", "¥", "¥1,234"},
		{"en-US", "1234.56", "USD", "USD1,234.56"},
		{"de-DE", "1234.56", "USD", "1.234,56" + nbsp + "USD"},
		{"en-US", "1234.56", "", "1,234.56"}, // no currency: plain number
	}
	for _, c := range cases {
		l, _ := Lookup(c.tag)
		if got := l.Money(c.digits, c.cur); got != c.want {
			t.Errorf("%s Money(%q,%q) = %q, want %q", c.tag, c.digits, c.cur, got, c.want)
		}
	}
}

// TestAccountingNegatives covers the parenthesised style finance apps often use.
func TestAccountingNegatives(t *testing.T) {
	l := Default
	l.Negative = Parentheses
	if got := l.Number("-1234.56"); got != "(1,234.56)" {
		t.Errorf("parenthesised = %q", got)
	}
	if got := l.Number("1234.56"); got != "1,234.56" {
		t.Errorf("positive should be unchanged, got %q", got)
	}
	l.Negative = MinusSuffix
	if got := l.Number("-1234.56"); got != "1,234.56-" {
		t.Errorf("suffix = %q", got)
	}
}

func TestDateOrders(t *testing.T) {
	cases := map[string]string{
		"en-US": "08/13/2026",
		"en-GB": "13/08/2026",
		"de-DE": "13.08.2026",
		"sv-SE": "2026-08-13",
		"ja-JP": "2026/08/13",
	}
	for tag, want := range cases {
		l, _ := Lookup(tag)
		if got := l.Date(2026, 8, 13); got != want {
			t.Errorf("%s Date = %q, want %q", tag, got, want)
		}
	}
}

func TestLookupFallback(t *testing.T) {
	// Exact.
	if l, ok := Lookup("de-DE"); !ok || l.Tag != "de-DE" {
		t.Errorf("de-DE = %v %v", l.Tag, ok)
	}
	// Language only resolves to that language's locale.
	if l, ok := Lookup("de"); !ok || l.Decimal != "," {
		t.Errorf("de fell back to %v (decimal %q)", l.Tag, l.Decimal)
	}
	// Underscores and case, as environment variables write them.
	if l, ok := Lookup("fr_FR"); !ok || l.Tag != "fr-FR" {
		t.Errorf("fr_FR = %v %v", l.Tag, ok)
	}
	// An unknown tag reports that it guessed, and still returns something usable.
	l, ok := Lookup("xx-YY")
	if ok {
		t.Error("an unknown tag should report a miss")
	}
	if l.Number("1234") != "1,234" {
		t.Errorf("fallback locale is unusable: %q", l.Number("1234"))
	}
	if _, ok := Lookup(""); ok {
		t.Error("an empty tag should report a miss")
	}
}

func TestAutoReadsEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_NUMERIC", "")
	t.Setenv("LANG", "de_DE.UTF-8")
	if l := Auto(); l.Tag != "de-DE" {
		t.Errorf("LANG=de_DE.UTF-8 gave %s", l.Tag)
	}

	// LC_ALL wins over LANG, and modifiers are stripped.
	t.Setenv("LC_ALL", "fr_FR.UTF-8@euro")
	if l := Auto(); l.Tag != "fr-FR" {
		t.Errorf("LC_ALL should win, got %s", l.Tag)
	}

	// The POSIX locale means "no preference", not a locale named C.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	if l := Auto(); l.Tag != Default.Tag {
		t.Errorf("C locale should fall back to the default, got %s", l.Tag)
	}
}

// TestGroupSeparatorIsNonBreaking checks the European space is the non-breaking
// one, so a number never wraps between its digits.
func TestGroupSeparatorIsNonBreaking(t *testing.T) {
	l, _ := Lookup("fr-FR")
	got := l.Number("1234567")
	if got != "1 234 567" {
		t.Errorf("fr-FR grouping = %q, want non-breaking spaces", got)
	}
	if got := l.Money("1234.56", "€"); got != "1 234,56 €" {
		t.Errorf("fr-FR money = %q, want a non-breaking space before the symbol", got)
	}
	if strings.ContainsRune(got, ' ') {
		t.Errorf("fr-FR money %q contains an ordinary space; digits could wrap", got)
	}
}
