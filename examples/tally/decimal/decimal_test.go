package decimal

import (
	"math/big"
	"testing"
)

func d(s string) Decimal { return RequireFromString(s) }

// TestZeroValueIsUsable is load-bearing: balances and category totals read
// Decimals out of maps, where an absent key yields the zero value, and add to
// them without a nil check.
func TestZeroValueIsUsable(t *testing.T) {
	var z Decimal
	if !z.IsZero() {
		t.Error("zero value is not zero")
	}
	if got := z.String(); got != "0" {
		t.Errorf("zero value String = %q, want %q", got, "0")
	}
	if got := z.Add(d("2.50")).String(); got != "2.5" {
		t.Errorf("zero + 2.50 = %q", got)
	}
	if got := z.Sub(d("1")).String(); got != "-1" {
		t.Errorf("zero - 1 = %q", got)
	}
	if !z.Mul(d("5")).IsZero() {
		t.Error("zero × 5 is not zero")
	}
	if z.IsNegative() || z.IsPositive() {
		t.Error("zero is neither negative nor positive")
	}
	if got := z.Abs().String(); got != "0" {
		t.Errorf("|zero| = %q", got)
	}
	if got := z.Neg().String(); got != "0" {
		t.Errorf("-zero = %q", got)
	}
	if got := z.StringFixed(2); got != "0.00" {
		t.Errorf("zero StringFixed(2) = %q", got)
	}
}

func TestParseAndString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0", "0"},
		{"1", "1"},
		{"-1", "-1"},
		{"+1", "1"},
		{"2400.00", "2400"}, // trailing fractional zeros are trimmed
		{"-2400.00", "-2400"},
		{"0.10", "0.1"},
		{"0.000001", "0.000001"},
		{".5", "0.5"},
		{"-.5", "-0.5"},
		{"123456789012345678.99", "123456789012345678.99"}, // beyond float64
		{"1.2e3", "1200"},
		{"5e-3", "0.005"},
		{" 42 ", "42"},
	}
	for _, c := range cases {
		got, err := NewFromString(c.in)
		if err != nil {
			t.Errorf("NewFromString(%q): %v", c.in, err)
			continue
		}
		if s := got.String(); s != c.want {
			t.Errorf("NewFromString(%q).String() = %q, want %q", c.in, s, c.want)
		}
	}

	for _, bad := range []string{"", "  ", "abc", "1.2.3", "1,234", "--1", "1e", "1e2e3"} {
		if _, err := NewFromString(bad); err == nil {
			t.Errorf("NewFromString(%q) should fail", bad)
		}
	}
}

// TestExponentSurvivesParsing pins the property tolerance inference depends on:
// "2400.00" was written to the cent and must report exponent −2, even though it
// prints as "2400". Normalising that away would silently widen every tolerance.
func TestExponentSurvivesParsing(t *testing.T) {
	cases := map[string]int32{
		"2400.00":  -2,
		"-2400.00": -2,
		"2400":     0,
		"0.005":    -3,
		"2.806":    -3,
		"1e3":      3,
	}
	for in, want := range cases {
		if got := d(in).Exponent(); got != want {
			t.Errorf("%q Exponent() = %d, want %d", in, got, want)
		}
	}
}

// TestArithmeticIsExact is the reason this type exists: the classic float
// failures must come out right.
func TestArithmeticIsExact(t *testing.T) {
	if got := d("0.1").Add(d("0.2")).String(); got != "0.3" {
		t.Errorf("0.1 + 0.2 = %q, want 0.3 (this is why money is not float64)", got)
	}
	// A hundred cents make a dollar, exactly.
	sum := Zero
	for i := 0; i < 100; i++ {
		sum = sum.Add(d("0.01"))
	}
	if got := sum.String(); got != "1" {
		t.Errorf("100 × 0.01 = %q, want 1", got)
	}
	// The fractional-share case from a real ledger: 2.806 units at 171.06.
	if got := d("2.806").Mul(d("171.06")).String(); got != "479.99436" {
		t.Errorf("2.806 × 171.06 = %q, want 479.99436", got)
	}
	if got := d("10").Sub(d("3.33")).Sub(d("3.33")).Sub(d("3.34")).String(); got != "0" {
		t.Errorf("a split to the cent did not balance: %q", got)
	}
}

func TestAddSubMulAcrossExponents(t *testing.T) {
	cases := []struct {
		a, op, b, want string
	}{
		{"1", "+", "0.001", "1.001"},
		{"1000", "+", "0.5", "1000.5"},
		{"1.5", "-", "1.50", "0"},
		{"-5", "+", "5.00", "0"},
		{"1.5", "*", "2", "3"},
		{"0.001", "*", "0.001", "0.000001"},
		{"-2", "*", "3.5", "-7"},
	}
	for _, c := range cases {
		var got Decimal
		switch c.op {
		case "+":
			got = d(c.a).Add(d(c.b))
		case "-":
			got = d(c.a).Sub(d(c.b))
		case "*":
			got = d(c.a).Mul(d(c.b))
		}
		if s := got.String(); s != c.want {
			t.Errorf("%s %s %s = %q, want %q", c.a, c.op, c.b, s, c.want)
		}
	}
}

// TestDiv covers the one inexact operation, including the price inversion the
// ledger does for every quoted rate.
func TestDiv(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"1", "2", "0.5"},
		{"10", "2", "5"},
		{"1", "3", "0.3333333333333333"},
		{"-1", "3", "-0.3333333333333333"},
		{"1", "-3", "-0.3333333333333333"},
		{"2", "3", "0.6666666666666667"}, // rounds half away from zero
		{"1", "1.10", "0.9090909090909091"},
		{"180", "1.10", "163.6363636363636364"},
	}
	for _, c := range cases {
		if got := d(c.a).Div(d(c.b)).String(); got != c.want {
			t.Errorf("%s / %s = %q, want %q", c.a, c.b, got, c.want)
		}
	}

	defer func() {
		if recover() == nil {
			t.Error("dividing by zero should panic")
		}
	}()
	_ = d("1").Div(Zero)
}

func TestCompare(t *testing.T) {
	// Equal across differing precision is what lets a balance assertion written
	// "100.00" match a computed 100.
	if !d("100.00").Equal(d("100")) {
		t.Error("100.00 != 100")
	}
	if d("100.01").Equal(d("100")) {
		t.Error("100.01 == 100")
	}
	if !d("-0.00").Equal(Zero) {
		t.Error("-0.00 != 0")
	}
	if !d("2").GreaterThan(d("1.99")) {
		t.Error("2 not > 1.99")
	}
	if !d("-2").LessThan(d("-1.99")) {
		t.Error("-2 not < -1.99")
	}
	if !d("1.5").LessThanOrEqual(d("1.50")) || !d("1.5").GreaterThanOrEqual(d("1.50")) {
		t.Error("1.5 not <= and >= 1.50")
	}
	if got := d("1").Cmp(d("2")); got != -1 {
		t.Errorf("Cmp = %d, want -1", got)
	}
}

func TestRoundAndStringFixed(t *testing.T) {
	cases := []struct {
		in     string
		places int32
		round  string
		fixed  string
	}{
		{"1.005", 2, "1.01", "1.01"}, // half away from zero, not banker's
		{"-1.005", 2, "-1.01", "-1.01"},
		{"1.004", 2, "1", "1.00"},
		{"2400", 2, "2400", "2400.00"},
		{"0.1", 2, "0.1", "0.10"},
		{"1.9999", 2, "2", "2.00"},
		{"-0.001", 2, "0", "0.00"},
		{"0.3333333333333333", 4, "0.3333", "0.3333"},
	}
	for _, c := range cases {
		if got := c.in; got == "" {
			continue
		}
		if got := d(c.in).Round(c.places).String(); got != c.round {
			t.Errorf("%s Round(%d) = %q, want %q", c.in, c.places, got, c.round)
		}
		if got := d(c.in).StringFixed(c.places); got != c.fixed {
			t.Errorf("%s StringFixed(%d) = %q, want %q", c.in, c.places, got, c.fixed)
		}
	}
}

func TestNewAndNewFromInt(t *testing.T) {
	if got := New(5, -3).String(); got != "0.005" {
		t.Errorf("New(5,-3) = %q, want 0.005", got)
	}
	if got := New(5, -1).String(); got != "0.5" {
		t.Errorf("New(5,-1) = %q", got)
	}
	if got := New(12, 2).String(); got != "1200" {
		t.Errorf("New(12,2) = %q", got)
	}
	if got := NewFromInt(-42).String(); got != "-42" {
		t.Errorf("NewFromInt(-42) = %q", got)
	}
}

func TestFloat64(t *testing.T) {
	f, ok := d("1234.56").Float64()
	if !ok || f != 1234.56 {
		t.Errorf("Float64 = %v %v", f, ok)
	}
	if got := d("-0.5").InexactFloat64(); got != -0.5 {
		t.Errorf("InexactFloat64 = %v", got)
	}
}

// TestNoAliasing guards a class of bug that would be invisible until a balance
// silently changed: operations must not mutate their operands' coefficients.
func TestNoAliasing(t *testing.T) {
	a := d("10")
	b := d("3")
	for i := 0; i < 3; i++ {
		_ = a.Add(b)
		_ = a.Sub(b)
		_ = a.Mul(b)
		_ = a.Neg()
		_ = a.Abs()
		_ = a.Round(1)
	}
	if got := a.String(); got != "10" {
		t.Errorf("operand mutated: a = %q, want 10", got)
	}
	if got := b.String(); got != "3" {
		t.Errorf("operand mutated: b = %q, want 3", got)
	}

	// Values sharing a coefficient must stay independent.
	shared := Decimal{coef: big.NewInt(100), exp: 0}
	other := shared
	_ = shared.Add(d("1"))
	if !other.Equal(d("100")) {
		t.Errorf("copy was mutated: %q", other.String())
	}
}

// TestRequireFromStringPanics: the convenience constructor is for literals, where
// a bad string is a bug rather than input.
func TestRequireFromStringPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("RequireFromString should panic on bad input")
		}
	}()
	_ = RequireFromString("not a number")
}
