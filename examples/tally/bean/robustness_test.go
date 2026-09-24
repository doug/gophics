package bean

import (
	"strings"
	"testing"
	"time"

	"github.com/doug/tally/decimal"
)

// Beancount writes a tolerance as "100.00 ~ 0.01 USD": the currency follows
// the tolerance. The parser used to stop at "~" and leave the amount's currency
// empty, so every tolerance-bearing assertion compared against nothing and
// failed.
func TestBalanceToleranceKeepsCurrency(t *testing.T) {
	src := `2021-01-01 open Assets:Cash USD
2021-01-01 open Equity:Opening USD
2021-01-02 * "Seed"
  Assets:Cash  100.00 USD
  Equity:Opening
2021-01-10 balance Assets:Cash 100.00 ~ 0.01 USD
`
	f := parseString(t, src)
	var a *Assertion
	for _, d := range f.Directives {
		if x, ok := d.(*Assertion); ok {
			a = x
		}
	}
	if a == nil {
		t.Fatal("no assertion parsed")
	}
	if a.Amount.Currency != "USD" {
		t.Errorf("assertion currency = %q, want USD", a.Amount.Currency)
	}
	if a.Tolerance == nil || !a.Tolerance.Equal(dec("0.01")) {
		t.Errorf("tolerance = %v, want 0.01", a.Tolerance)
	}
	l, _ := LoadString("t", src)
	if len(l.Problems) != 0 {
		t.Errorf("a satisfied assertion was reported: %v", l.Problems)
	}
}

// One byte the lexer does not know — a '|' in an org-mode heading, a stray
// quote — used to abort the whole parse and leave the ledger unopenable. It
// costs that line now, and the rest of the file still loads.
func TestStrayCharactersCostOneLine(t *testing.T) {
	for _, src := range []string{
		"* 2021 | Q1\n2021-01-01 open Assets:Cash USD\n",
		"* Doug's \"notes\n2021-01-01 open Assets:Cash USD\n",
		"2021-01-01 open Assets:Cash USD\n[ not beancount ]\n",
	} {
		first := strings.SplitN(src, "\n", 2)[0]
		f, err := Parse("t.beancount", src)
		if f == nil {
			t.Errorf("%q: Parse returned no file", first)
			continue
		}
		if err == nil {
			t.Errorf("%q: the bad line was not reported", first)
		} else if !strings.Contains(err.Error(), "t.beancount:") {
			t.Errorf("%q: error %q does not name the file", first, err)
		}
		opens := 0
		for _, d := range f.Directives {
			if _, ok := d.(*Open); ok {
				opens++
			}
		}
		if opens != 1 {
			t.Errorf("%q: %d open directives survived, want 1", first, opens)
		}
		l, _ := LoadString("t.beancount", src)
		if l == nil {
			t.Errorf("%q: LoadString returned no ledger", first)
		} else if _, ok := l.Account("Assets:Cash"); !ok {
			t.Errorf("%q: the account on the good line is missing", first)
		}
	}
}

// A pad's synthesized transaction is only known when its balance assertion is
// reached, after every transaction in between has posted. It must still land
// at its own date in the account's postings, because those are documented as
// date-ordered and the register's running balance is computed in that order.
func TestPadPostsInDateOrder(t *testing.T) {
	l, _ := LoadString("t", `2021-01-01 open Assets:Cash USD
2021-01-01 open Equity:Opening USD
2021-01-01 open Expenses:Food USD
2021-01-01 pad Assets:Cash Equity:Opening
2021-01-05 * "Lunch"
  Expenses:Food  10.00 USD
  Assets:Cash
2021-01-10 balance Assets:Cash 90.00 USD
`)
	if len(l.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", l.Problems)
	}
	for _, name := range []Account{"Assets:Cash", "Equity:Opening"} {
		a, ok := l.Account(name)
		if !ok {
			t.Fatalf("no account %s", name)
		}
		for i := 1; i < len(a.Postings); i++ {
			if a.Postings[i].Txn.When().Before(a.Postings[i-1].Txn.When()) {
				t.Errorf("%s: posting %d (%s) comes after %s", name, i,
					a.Postings[i].Txn.When(), a.Postings[i-1].Txn.When())
			}
		}
		if len(a.Postings) > 0 && a.Postings[0].Txn.Flag != "P" {
			t.Errorf("%s: first posting is %q, want the pad", name, a.Postings[0].Txn.Narration)
		}
	}
}

// An entry inserted into a CRLF file must use CRLF: a mixed-ending file parses,
// but diffs badly and is exactly what a Windows editor then "fixes" wholesale.
func TestInsertKeepsFileLineEndings(t *testing.T) {
	src := "2021-01-01 open Assets:Cash USD\r\n2021-01-01 open Expenses:Food USD\r\n2021-01-02 * \"A\"\r\n  Assets:Cash -1 USD\r\n  Expenses:Food 1 USD\r\n"
	s, err := NewSource("t", src)
	if err != nil {
		t.Fatal(err)
	}
	if s.String() != src {
		t.Fatal("the text did not round-trip before any edit")
	}
	txn := NewTransaction(Date{Year: 2021, Month: 1, Day: 3}, "*", "", "B",
		&Posting{Account: "Assets:Cash", Amount: &Amount{Number: decimal.RequireFromString("-2"), Currency: "USD"}},
		&Posting{Account: "Expenses:Food", Amount: &Amount{Number: decimal.RequireFromString("2"), Currency: "USD"}})
	s.Insert(txn, FormatOptions{})
	out := s.String()
	crlf := strings.Count(out, "\r\n")
	if lf := strings.Count(out, "\n") - crlf; lf != 0 {
		t.Errorf("after Insert: %d CRLF lines and %d bare-LF lines", crlf, lf)
	}
	if strings.Contains(out, "\r\r") {
		t.Error("a carriage return was doubled")
	}
	// An LF file stays LF.
	s2, _ := NewSource("t", strings.ReplaceAll(src, "\r\n", "\n"))
	s2.Insert(txn, FormatOptions{})
	if strings.Contains(s2.String(), "\r") {
		t.Error("a CR crept into an LF file")
	}
}

// Sorted is stable and handles a newest-first ledger without the quadratic
// pass the old insertion sort took on it.
func TestSortedByDateNewestFirst(t *testing.T) {
	var b strings.Builder
	b.WriteString("2000-01-01 open Assets:Cash USD\n2000-01-01 open Expenses:X USD\n")
	const n = 3000
	for i := n; i >= 1; i-- {
		d := Date{Year: 2001 + i/365, Month: time.Month(1 + (i/28)%12), Day: 1 + i%28}
		b.WriteString(d.String() + " * \"t\"\n  Assets:Cash  -1.00 USD\n  Expenses:X    1.00 USD\n")
	}
	f := parseString(t, b.String())
	sorted := f.Sorted()
	for i := 1; i < len(sorted); i++ {
		if sorted[i].When().Before(sorted[i-1].When()) {
			t.Fatalf("out of order at %d", i)
		}
	}
	if len(sorted) != n+2 {
		t.Fatalf("%d directives, want %d", len(sorted), n+2)
	}
}
