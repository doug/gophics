package intl

import "strings"

// Per-currency fraction digits.
//
// Locale patterns decide *where* the symbol and separators go; the currency
// decides *how many* fraction digits there are, and the two are independent.
// ¥1,235 and $1,234.56 are both correct in en-US; "¥1,234.56" is not, and
// neither is "KWD 1.235" for a currency with three. A formatter that assumes
// two is wrong for about a fifth of the world's currencies and for the two
// largest Asian ones, so getting the locale pattern right and the digit count
// wrong still produces a visibly foreign number.
//
// # Where this data comes from, and how to check it
//
// CLDR's supplemental/currencyData.xml <fractions> block, as exposed by
// golang.org/x/text/currency's standard rounding scale. The list below is
// every code in that file whose digit count is not the default of 2, and
// TestCurrencyDigitsMatchCLDR in cldr_test.go checks all ~300 ISO codes x/text
// knows against it on every run — including the ones this table omits, which
// is what verifies that 2 really is the default rather than a guess.
//
// x/text/currency is a test-only import for the same reason the plural tables
// are: its tables cost ~403KB of wasm, and this map costs about 400 bytes.
//
// # CLDR digits, not ISO 4217 minor units
//
// These are deliberately CLDR's display digits, which disagree with ISO 4217's
// minor-unit counts for a handful of currencies — ISK, MUR and PKR among them,
// where ISO records 2 and CLDR shows 0 because the subunit is no longer in
// circulation. This package formats for a reader, so CLDR is the right source;
// an accounting system that must store and reconcile minor units should take
// ISO's numbers and pass the digit count it wants to Quantize explicitly.
//
// Cash rounding (the Swiss 5-rappen rule, Canada's nickel) is a separate CLDR
// field and is not handled here: it changes the *value*, not its spelling, and
// this package never changes a value.
var currencyDigits = map[string]int{
	// Zero-digit currencies.
	"ADP": 0, "AFN": 0, "ALL": 0, "AMD": 0, "BIF": 0, "BYR": 0, "CLP": 0,
	"COP": 0, "DJF": 0, "ESP": 0, "GNF": 0, "GYD": 0, "IDR": 0, "IQD": 0,
	"IRR": 0, "ISK": 0, "ITL": 0, "JPY": 0, "KMF": 0, "KPW": 0, "KRW": 0,
	"LAK": 0, "LBP": 0, "LUF": 0, "MGA": 0, "MGF": 0, "MMK": 0, "MNT": 0,
	"MRO": 0, "MUR": 0, "PKR": 0, "PYG": 0, "RSD": 0, "RWF": 0, "SLL": 0,
	"SOS": 0, "STD": 0, "SYP": 0, "TMM": 0, "TRL": 0, "TZS": 0, "UGX": 0,
	"UYI": 0, "UZS": 0, "VND": 0, "VUV": 0, "XAF": 0, "XOF": 0, "XPF": 0,
	"YER": 0, "ZMK": 0, "ZWD": 0,

	// Three-digit currencies: the Gulf and North African dinars, whose
	// subunits are thousandths.
	"BHD": 3, "JOD": 3, "KWD": 3, "LYD": 3, "OMR": 3, "TND": 3,

	// Four digits: a Chilean unit of account, not a circulating currency.
	"CLF": 4,
}

// CurrencyDigits reports how many fraction digits a currency is written with.
//
// The code is an ISO 4217 alphabetic code, case-insensitive. An unknown code
// gets 2, which is both the overwhelming majority and CLDR's own default, so a
// currency added after this table was written formats plausibly rather than
// not at all.
func CurrencyDigits(code string) int {
	if d, ok := currencyDigits[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return d
	}
	return 2
}

// Currency formats an amount in a currency, using that currency's own fraction
// digit count and this locale's symbol placement and spacing.
//
// symbol is what to show — "$", "€", "¥" — and the ISO code is used when it is
// empty. The symbol is the caller's because CLDR's choice of symbol depends on
// both the currency *and* the locale reading it: CAD is "$" to a Canadian and
// "CA$" to an American, and a table that got that backwards would be worse than
// no table. Shipping the ~60KB of CLDR symbol data to every app to answer it
// is not a trade this package makes; an app that needs it can take
// golang.org/x/text/currency directly.
//
// The amount is quantized, not merely re-punctuated: Currency("5", "USD", "$")
// is "$5.00" and Currency("1234.56", "JPY", "¥") is "¥1,235". That is the one
// place in this package where digits change, and it is the point of the
// function — a caller that wants its own precision respected should use Money.
func (l Locale) Currency(digits, code, symbol string) string {
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(code))
	}
	return l.Money(Quantize(digits, CurrencyDigits(code)), symbol)
}

// Quantize rewrites a decimal string to exactly n fraction digits, padding with
// zeros or rounding half away from zero.
//
// Half away from zero is the rule every currency convention uses for display
// (0.125 → 0.13, -0.125 → -0.13), and it is not what Go's float formatting
// does, which rounds half to even. Done on the digit string so that a value too
// large or too precise for a float64 survives: that is why this package takes
// strings in the first place, and a Quantize that round-tripped through a float
// would quietly undo it.
func Quantize(digits string, n int) string {
	if n < 0 {
		n = 0
	}
	neg, intPart, frac := split(digits)

	switch {
	case len(frac) < n:
		frac += strings.Repeat("0", n-len(frac))
	case len(frac) > n:
		// Look at the first dropped digit to decide whether to round up.
		roundUp := frac[n] >= '5'
		frac = frac[:n]
		if roundUp {
			intPart, frac = increment(intPart, frac)
		}
	}

	s := intPart
	if n > 0 {
		s += "." + frac
	}
	if neg && !allZero(s) {
		// A value that rounds to zero must not keep its sign: "-0.00" reads as
		// a debt of nothing, and parentheses would make it "(0.00)".
		s = "-" + s
	}
	return s
}

// increment adds one at the least significant position of intPart+frac,
// carrying across the decimal point. String arithmetic rather than integer
// arithmetic because the integer part may be longer than any machine word —
// the package's whole premise is that it does not re-derive the value.
func increment(intPart, frac string) (string, string) {
	b := []byte(intPart + frac)
	i := len(b) - 1
	for ; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			break
		}
		b[i] = '0'
	}
	if i < 0 {
		b = append([]byte{'1'}, b...)
		return string(b[:len(intPart)+1]), string(b[len(intPart)+1:])
	}
	return string(b[:len(intPart)]), string(b[len(intPart):])
}

func allZero(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '1' && s[i] <= '9' {
			return false
		}
	}
	return true
}
