package bean

import (
	"sort"

	"github.com/shopspring/decimal"
)

// PostingRef pairs a posting with the transaction it came from — an account's
// history is a list of these, which is what a register displays.
type PostingRef struct {
	Txn     *Transaction
	Posting *Posting
}

// AccountInfo is an account's processed state: when it opened and closed, what it
// may hold, and every posting against it in date order.
type AccountInfo struct {
	Name       Account
	Open       *Date
	Close      *Date
	Currencies []string
	Booking    string
	Meta       Meta
	Postings   []PostingRef
}

// IsOpenOn reports whether the account accepts postings on a date. An account with
// no open directive is treated as open — beancount can be run in a strict mode
// that requires them, but refusing to show a user's data because a declaration is
// missing is the wrong default for an app.
func (a *AccountInfo) IsOpenOn(d Date) bool {
	if a.Open != nil && d.Before(*a.Open) {
		return false
	}
	if a.Close != nil && d.After(*a.Close) {
		return false
	}
	return true
}

// Ledger is a processed set of directives: accounts with their histories and
// balances, price data, and whatever problems processing found.
type Ledger struct {
	Directives []Directive
	Options    map[string]string

	accounts map[Account]*AccountInfo
	prices   *priceGraph

	// Problems are the semantic errors found while processing (unbalanced
	// transactions, failed assertions). They do not stop processing: a ledger with
	// a bad assertion should still open and show its data, with the problem
	// surfaced rather than the file refused.
	Problems ErrorList
}

// Process turns parsed directives into a Ledger.
func Process(dirs []Directive) *Ledger {
	l := &Ledger{
		Directives: dirs,
		Options:    map[string]string{},
		accounts:   map[Account]*AccountInfo{},
		prices:     newPriceGraph(),
	}

	// Directives are applied in date order regardless of how the file lists them,
	// because a balance assertion must see every transaction dated before it.
	sorted := make([]Directive, len(dirs))
	copy(sorted, dirs)
	stableSortByDate(sorted)

	// A pad waits for the next balance assertion on its account to tell it how
	// much to insert.
	pending := map[Account]*Pad{}

	for _, d := range sorted {
		switch v := d.(type) {
		case *Option:
			l.Options[v.Name] = v.Value

		case *Open:
			a := l.account(v.Account)
			date := v.Date
			a.Open = &date
			a.Currencies = v.Currencies
			a.Booking = v.Booking
			a.Meta = v.Meta

		case *Close:
			a := l.account(v.Account)
			date := v.Date
			a.Close = &date

		case *Price:
			l.prices.add(v.Date, v.Currency, v.Amount.Currency, v.Amount.Number)

		case *Pad:
			pending[v.Account] = v

		case *Assertion:
			l.checkAssertion(v, pending)

		case *Transaction:
			if err := v.Balance(); err != nil {
				l.Problems = append(l.Problems, err)
				// Even an unbalanced transaction is recorded: the user needs to
				// see it in the register in order to fix it.
			}
			l.post(v)
		}
	}
	return l
}

// post files a transaction's postings against their accounts.
func (l *Ledger) post(t *Transaction) {
	for _, p := range t.Postings {
		if p.Amount == nil {
			continue
		}
		a := l.account(p.Account)
		a.Postings = append(a.Postings, PostingRef{Txn: t, Posting: p})
	}
}

// checkBalance verifies an assertion, first satisfying any pending pad.
//
// A balance assertion is written as of the *start* of its date, so it covers
// postings strictly before it — the usual reading of "on the morning of the 5th".
func (l *Ledger) checkAssertion(b *Assertion, pending map[Account]*Pad) {
	actual := l.balanceBefore(b.Account, b.Date).Get(b.Amount.Currency)
	diff := b.Amount.Number.Sub(actual)

	if pad, ok := pending[b.Account]; ok {
		delete(pending, b.Account)
		if !diff.IsZero() {
			l.insertPad(pad, b, diff)
			return // the pad makes the assertion true by construction
		}
	}

	tol := b.Amount.Number.Abs().Mul(decimal.Zero) // start from zero, then widen
	if b.Tolerance != nil {
		tol = *b.Tolerance
	} else if exp := b.Amount.Number.Exponent(); exp < 0 {
		tol = decimal.New(5, exp-1) // half the least significant digit
	}
	if diff.Abs().GreaterThan(tol) {
		l.Problems = append(l.Problems, &AssertionError{
			Pos: b.Where(), Account: b.Account, Expected: b.Amount,
			Actual: actual, Diff: diff,
		})
	}
}

// insertPad synthesizes the transaction a pad directive stands for: move exactly
// enough from the source account to make the following assertion hold.
func (l *Ledger) insertPad(pad *Pad, b *Assertion, diff decimal.Decimal) {
	txn := &Transaction{
		base:      base{Date: pad.Date, Pos: pad.Where()},
		Flag:      "P",
		Narration: "(padding inserted for balance of " + b.Amount.String() + ")",
		Postings: []*Posting{
			{Account: pad.Account, Amount: &Amount{Number: diff, Currency: b.Amount.Currency}, Inferred: true},
			{Account: pad.Source, Amount: &Amount{Number: diff.Neg(), Currency: b.Amount.Currency}, Inferred: true},
		},
	}
	l.Directives = append(l.Directives, txn)
	l.post(txn)
}

// account returns the record for name, creating it on first use so that posting
// to an undeclared account still works.
func (l *Ledger) account(name Account) *AccountInfo {
	if a, ok := l.accounts[name]; ok {
		return a
	}
	a := &AccountInfo{Name: name}
	l.accounts[name] = a
	return a
}

// Account returns one account's record.
func (l *Ledger) Account(name Account) (*AccountInfo, bool) {
	a, ok := l.accounts[name]
	return a, ok
}

// Accounts returns every account, sorted by name.
func (l *Ledger) Accounts() []*AccountInfo {
	out := make([]*AccountInfo, 0, len(l.accounts))
	for _, a := range l.accounts {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AccountNames returns every account name, sorted.
func (l *Ledger) AccountNames() []Account {
	out := make([]Account, 0, len(l.accounts))
	for name := range l.accounts {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// BalanceOf returns an account's balance across all of its postings.
func (l *Ledger) BalanceOf(name Account) Balance {
	b := NewBalance()
	if a, ok := l.accounts[name]; ok {
		for _, ref := range a.Postings {
			b.AddAmount(*ref.Posting.Amount)
		}
	}
	return b
}

// balanceBefore returns an account's balance from postings strictly before a date.
func (l *Ledger) balanceBefore(name Account, d Date) Balance {
	b := NewBalance()
	a, ok := l.accounts[name]
	if !ok {
		return b
	}
	for _, ref := range a.Postings {
		if ref.Txn.When().Before(d) {
			b.AddAmount(*ref.Posting.Amount)
		}
	}
	return b
}

// BalanceAsOf returns an account's balance including everything up to and
// including a date — the point-in-time reading a balance sheet wants.
func (l *Ledger) BalanceAsOf(name Account, d Date) Balance {
	b := NewBalance()
	a, ok := l.accounts[name]
	if !ok {
		return b
	}
	for _, ref := range a.Postings {
		if !ref.Txn.When().After(d) {
			b.AddAmount(*ref.Posting.Amount)
		}
	}
	return b
}

// BalanceBetween returns the net change to an account over [from, to] — the
// period reading an income statement wants.
func (l *Ledger) BalanceBetween(name Account, from, to Date) Balance {
	b := NewBalance()
	a, ok := l.accounts[name]
	if !ok {
		return b
	}
	for _, ref := range a.Postings {
		d := ref.Txn.When()
		if !d.Before(from) && !d.After(to) {
			b.AddAmount(*ref.Posting.Amount)
		}
	}
	return b
}

// Span reports the first and last transaction dates, and whether there are any.
func (l *Ledger) Span() (first, last Date, ok bool) {
	for _, d := range l.Directives {
		if _, isTxn := d.(*Transaction); !isTxn {
			continue
		}
		when := d.When()
		if !ok {
			first, last, ok = when, when, true
			continue
		}
		if when.Before(first) {
			first = when
		}
		if when.After(last) {
			last = when
		}
	}
	return first, last, ok
}

// AssertionError reports a balance assertion that did not hold.
type AssertionError struct {
	Pos      Position
	Account  Account
	Expected Amount
	Actual   decimal.Decimal
	Diff     decimal.Decimal
}

func (e *AssertionError) Error() string {
	return e.Pos.String() + ": balance assertion failed for " + string(e.Account) +
		": expected " + e.Expected.String() +
		", found " + e.Actual.String() + " " + e.Expected.Currency +
		" (off by " + e.Diff.String() + ")"
}
