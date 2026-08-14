// Package decimal is exact decimal arithmetic for money.
//
// Money cannot be a float64: 0.1 + 0.2 is not 0.3 in binary floating point, and a
// ledger that has to balance to the cent will not. A value here is an arbitrary-
// precision integer coefficient with a power-of-ten exponent, so every number a
// user can write is represented exactly and addition never invents a fraction of
// a cent.
//
// It replaces a third-party dependency with about three hundred lines over
// math/big, matching the subset Tally uses. The behaviours worth knowing:
//
//   - The zero value is a usable zero, so a Decimal read from an absent map key
//     can be added to without a nil check.
//   - The exponent survives parsing: "2400.00" keeps exp -2 even though it prints
//     as "2400". Tolerances are derived from the precision the author wrote, so
//     that distinction carries meaning and must not be normalised away.
//   - String trims trailing fractional zeros; StringFixed pads to a fixed width,
//     which is what a column of figures wants.
package decimal

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// DivisionPrecision is how many fractional digits Div produces. Division is the
// one operation that cannot always be exact (1/3 has no finite decimal form), so
// it is the one place a precision has to be chosen.
const DivisionPrecision = 16

// Decimal is an exact decimal number: coef × 10^exp.
//
// The zero value is zero. coef is nil for zero rather than a big.Int, so a
// Decimal costs nothing until it holds a value.
type Decimal struct {
	coef *big.Int
	exp  int32
}

// Zero is the zero value, provided for symmetry with the constructors.
var Zero = Decimal{}

var ten = big.NewInt(10)

// New returns value × 10^exp — New(5, -3) is 0.005.
func New(value int64, exp int32) Decimal {
	return Decimal{coef: big.NewInt(value), exp: exp}
}

// NewFromInt returns a whole number.
func NewFromInt(v int64) Decimal { return Decimal{coef: big.NewInt(v), exp: 0} }

// NewFromString parses a decimal literal: an optional sign, digits, an optional
// fraction, and an optional exponent ("1234", "-0.05", "1.2e3").
func NewFromString(s string) (Decimal, error) {
	str := strings.TrimSpace(s)
	if str == "" {
		return Decimal{}, errors.New("decimal: empty string")
	}

	// An exponent suffix shifts the result; the mantissa is parsed as usual.
	var expShift int32
	if i := strings.IndexAny(str, "eE"); i >= 0 {
		n, err := strconv.ParseInt(str[i+1:], 10, 32)
		if err != nil {
			return Decimal{}, errors.New("decimal: bad exponent in " + strconv.Quote(s))
		}
		expShift = int32(n)
		str = str[:i]
	}

	neg := false
	switch {
	case strings.HasPrefix(str, "-"):
		neg, str = true, str[1:]
	case strings.HasPrefix(str, "+"):
		str = str[1:]
	}

	intPart, fracPart := str, ""
	if i := strings.IndexByte(str, '.'); i >= 0 {
		intPart, fracPart = str[:i], str[i+1:]
	}
	digits := intPart + fracPart
	if digits == "" || !allDigits(digits) {
		return Decimal{}, errors.New("decimal: cannot parse " + strconv.Quote(s))
	}

	coef, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Decimal{}, errors.New("decimal: cannot parse " + strconv.Quote(s))
	}
	if neg {
		coef.Neg(coef)
	}
	return Decimal{coef: coef, exp: expShift - int32(len(fracPart))}, nil
}

// RequireFromString parses and panics on failure — for literals in tests and
// constants, where a bad string is a programming error rather than input.
func RequireFromString(s string) Decimal {
	d, err := NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// value returns the coefficient, treating nil as zero, without aliasing.
func (d Decimal) value() *big.Int {
	if d.coef == nil {
		return new(big.Int)
	}
	return d.coef
}

// Exponent returns the power of ten the coefficient is scaled by. It reflects the
// precision the value was written with, which is why parsing does not normalise
// it: "2400.00" reports -2, and a tolerance derived from it means "to the cent".
func (d Decimal) Exponent() int32 { return d.exp }

// rescale returns an equal value expressed at the given (smaller) exponent.
func (d Decimal) rescale(exp int32) *big.Int {
	v := new(big.Int).Set(d.value())
	if diff := d.exp - exp; diff > 0 {
		v.Mul(v, pow10(diff))
	}
	return v
}

// align expresses two values at a common exponent — the smaller of the two, so
// neither loses digits.
func align(a, b Decimal) (x, y *big.Int, exp int32) {
	exp = a.exp
	if b.exp < exp {
		exp = b.exp
	}
	return a.rescale(exp), b.rescale(exp), exp
}

func pow10(n int32) *big.Int {
	return new(big.Int).Exp(ten, big.NewInt(int64(n)), nil)
}

// Add returns d + o.
func (d Decimal) Add(o Decimal) Decimal {
	x, y, exp := align(d, o)
	return Decimal{coef: x.Add(x, y), exp: exp}
}

// Sub returns d − o.
func (d Decimal) Sub(o Decimal) Decimal {
	x, y, exp := align(d, o)
	return Decimal{coef: x.Sub(x, y), exp: exp}
}

// Mul returns d × o, exactly: the exponents add and the coefficients multiply, so
// no precision is lost.
func (d Decimal) Mul(o Decimal) Decimal {
	return Decimal{
		coef: new(big.Int).Mul(d.value(), o.value()),
		exp:  d.exp + o.exp,
	}
}

// Div returns d ÷ o to DivisionPrecision fractional digits, rounded half away
// from zero. Dividing by zero panics, as it does for integers.
func (d Decimal) Div(o Decimal) Decimal {
	if o.IsZero() {
		panic("decimal: division by zero")
	}
	// d/o = (coefD/coefO) × 10^(expD−expO), and the result is wanted at exponent
	// −prec, so the quotient must be coefD × 10^(prec + expD − expO) / coefO.
	shift := int32(DivisionPrecision) + d.exp - o.exp
	num := new(big.Int).Set(d.value())
	den := new(big.Int).Set(o.value())
	if shift > 0 {
		num.Mul(num, pow10(shift))
	} else if shift < 0 {
		den.Mul(den, pow10(-shift))
	}
	q := divRoundHalfAway(num, den)
	return Decimal{coef: q, exp: -DivisionPrecision}
}

// divRoundHalfAway divides and rounds the quotient half away from zero, which is
// what people expect of money and what the previous implementation did.
func divRoundHalfAway(num, den *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	// Round up when twice the remainder reaches the divisor.
	r2 := new(big.Int).Abs(r)
	r2.Lsh(r2, 1)
	if r2.CmpAbs(den) >= 0 {
		if (num.Sign() < 0) != (den.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// Neg returns −d.
func (d Decimal) Neg() Decimal {
	return Decimal{coef: new(big.Int).Neg(d.value()), exp: d.exp}
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	return Decimal{coef: new(big.Int).Abs(d.value()), exp: d.exp}
}

// Cmp compares two values numerically: −1, 0 or +1. Values written with different
// precision compare equal when they are equal ("2400" and "2400.00").
func (d Decimal) Cmp(o Decimal) int {
	x, y, _ := align(d, o)
	return x.Cmp(y)
}

func (d Decimal) Equal(o Decimal) bool              { return d.Cmp(o) == 0 }
func (d Decimal) GreaterThan(o Decimal) bool        { return d.Cmp(o) > 0 }
func (d Decimal) GreaterThanOrEqual(o Decimal) bool { return d.Cmp(o) >= 0 }
func (d Decimal) LessThan(o Decimal) bool           { return d.Cmp(o) < 0 }
func (d Decimal) LessThanOrEqual(o Decimal) bool    { return d.Cmp(o) <= 0 }

// IsZero reports whether the value is exactly zero, at any precision.
func (d Decimal) IsZero() bool { return d.value().Sign() == 0 }

// IsNegative reports a value below zero (zero is neither negative nor positive).
func (d Decimal) IsNegative() bool { return d.value().Sign() < 0 }

// IsPositive reports a value above zero.
func (d Decimal) IsPositive() bool { return d.value().Sign() > 0 }

// Sign returns −1, 0 or +1.
func (d Decimal) Sign() int { return d.value().Sign() }

// Round returns the value rounded to places decimal digits, half away from zero.
func (d Decimal) Round(places int32) Decimal {
	if d.exp >= -places {
		return d // already coarser than the requested precision
	}
	den := pow10(-places - d.exp)
	q := divRoundHalfAway(new(big.Int).Set(d.value()), den)
	return Decimal{coef: q, exp: -places}
}

// String renders the value with trailing fractional zeros removed, so "2400.00"
// prints as "2400". Use StringFixed for a column of aligned figures.
func (d Decimal) String() string {
	s := d.text(-d.exp)
	if !strings.ContainsRune(s, '.') {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// StringFixed renders the value with exactly places decimal digits, rounding when
// the value is finer and padding when it is coarser.
func (d Decimal) StringFixed(places int32) string {
	return d.Round(places).text(places)
}

// text renders the value with exactly n fractional digits, assuming the value has
// already been rounded to at most n.
func (d Decimal) text(n int32) string {
	if n < 0 {
		n = 0
	}
	// Express the coefficient at exponent −n.
	v := new(big.Int).Set(d.value())
	if shift := d.exp + n; shift > 0 {
		v.Mul(v, pow10(shift))
	} else if shift < 0 {
		v = divRoundHalfAway(v, pow10(-shift))
	}

	neg := v.Sign() < 0
	digits := new(big.Int).Abs(v).String()
	if n == 0 {
		return sign(neg) + digits
	}
	for int32(len(digits)) <= n { // pad so there is at least "0.xxx"
		digits = "0" + digits
	}
	cut := int32(len(digits)) - n
	return sign(neg) + digits[:cut] + "." + digits[cut:]
}

func sign(neg bool) string {
	if neg {
		return "-"
	}
	return ""
}

// Float64 returns the value as a float64 and whether it is exact. Use it for
// drawing — a chart's pixels are approximate anyway — never for arithmetic.
func (d Decimal) Float64() (float64, bool) {
	f, err := strconv.ParseFloat(d.String(), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// InexactFloat64 returns the value as a float64, discarding the exactness flag.
func (d Decimal) InexactFloat64() float64 {
	f, _ := d.Float64()
	return f
}
