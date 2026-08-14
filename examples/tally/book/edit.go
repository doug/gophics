package book

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/doug/tally/decimal"

	"github.com/doug/tally/bean"
)

// NewEntry is a transaction the user is adding: money moving from one account to
// another. Two postings covers the overwhelming majority of hand-entered
// transactions (a purchase, a transfer, a paycheck deposit); splits are a later
// refinement, and the engine already supports them.
type NewEntry struct {
	Date      time.Time
	Payee     string
	Narration string
	// From is credited (money leaves it) and To is debited, so a purchase reads
	// naturally: from Assets:Checking to Expenses:Food.
	From, To string
	Amount   decimal.Decimal
	Currency string
}

// Validate reports what is missing or wrong, in the order a form should complain.
func (e NewEntry) Validate(known []string) error {
	switch {
	case e.Date.IsZero():
		return errors.New("pick a date")
	case strings.TrimSpace(e.From) == "":
		return errors.New("choose the account the money comes from")
	case strings.TrimSpace(e.To) == "":
		return errors.New("choose the account the money goes to")
	case e.From == e.To:
		return errors.New("the two accounts must differ")
	case e.Amount.IsZero():
		return errors.New("enter an amount")
	case e.Amount.IsNegative():
		return errors.New("enter a positive amount; the direction comes from the accounts")
	case strings.TrimSpace(e.Currency) == "":
		return errors.New("enter a currency")
	}
	return nil
}

// AddResult reports what an edit did, including consequences the user needs to
// know about but did not ask for.
type AddResult struct {
	// Line is where the entry landed in the file.
	Line int
	// Invalidated lists balance assertions that no longer hold because of this
	// edit. Inserting money movement genuinely changes every later checkpoint on
	// that account, so this is expected rather than an error — but silently
	// leaving a user's ledger failing its own checks would be indefensible.
	Invalidated []string
	// Saved reports whether the change reached disk.
	Saved bool
}

// Add inserts a transaction and, when the ledger came from a real file, writes it
// back. The in-memory ledger is reprocessed either way, so the UI reflects the
// change immediately.
func (b *Book) Add(e NewEntry) (AddResult, error) {
	var res AddResult
	if b.src == nil {
		return res, errors.New("this ledger is read-only")
	}
	if err := e.Validate(b.AccountNames()); err != nil {
		return res, err
	}

	d := bean.NewDate(e.Date)
	amount := func(n decimal.Decimal) *bean.Amount {
		return &bean.Amount{Number: n, Currency: e.Currency, Raw: n.StringFixed(2)}
	}
	txn := bean.NewTransaction(d, "*", e.Payee, e.Narration,
		&bean.Posting{Account: bean.Account(e.From), Amount: amount(e.Amount.Neg())},
		&bean.Posting{Account: bean.Account(e.To), Amount: amount(e.Amount)},
	)

	before := b.assertionFailures()
	res.Line = b.src.Insert(txn, bean.FormatOptions{})

	led, err := b.src.Ledger()
	if err != nil {
		return res, err
	}
	b.led = led
	res.Invalidated = newFailures(before, b.assertionFailures())

	if b.writable() {
		if err := os.WriteFile(b.Path, b.src.Bytes(), 0o644); err != nil {
			return res, err
		}
		res.Saved = true
	}
	return res, nil
}

// CanEdit reports whether this ledger can be added to at all (an embedded demo
// ledger can be edited in memory but never saved).
func (b *Book) CanEdit() bool { return b.src != nil }

// writable reports whether the ledger came from a real file we can write back to.
func (b *Book) writable() bool {
	if b.Path == "" {
		return false
	}
	_, err := os.Stat(b.Path)
	return err == nil
}

// Writable reports whether saving is possible, for the UI to label its button.
func (b *Book) Writable() bool { return b.src != nil && b.writable() }

// assertionFailures describes every currently-failing balance assertion.
func (b *Book) assertionFailures() map[string]bool {
	out := map[string]bool{}
	for _, p := range b.led.Problems {
		if _, ok := p.(*bean.AssertionError); ok {
			out[p.Error()] = true
		}
	}
	return out
}

// newFailures returns the assertion failures present after an edit but not before,
// so the user is told about consequences of *their* change and not pre-existing
// problems they have already seen.
func newFailures(before, after map[string]bool) []string {
	var out []string
	for msg := range after {
		if !before[msg] {
			out = append(out, msg)
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
