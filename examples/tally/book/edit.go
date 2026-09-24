package book

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// Validate reports what is missing or wrong, in the order a form should
// complain. known is the ledger's account list: an entry to an account that is
// not in it is refused, because beancount would otherwise create the account
// on the spot and a typo would become a new account without anyone noticing.
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
	case !slices.Contains(known, e.From):
		return fmt.Errorf("%s is not an account in this ledger", e.From)
	case !slices.Contains(known, e.To):
		return fmt.Errorf("%s is not an account in this ledger", e.To)
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
//
// The file is written before the in-memory ledger is swapped, and the text is
// put back as it was if the write fails: an entry that reached memory but not
// disk would show as added, and pressing Save again would insert it twice.
// The write itself goes through a temporary file and a rename, so a crash
// mid-write cannot leave the ledger truncated, and it is refused when the
// file has changed on disk since it was opened rather than overwriting an
// edit made elsewhere.
func (b *Book) Add(e NewEntry) (AddResult, error) {
	var res AddResult
	if b.src == nil {
		return res, errors.New("this ledger is read-only")
	}
	if err := e.Validate(b.AccountNames()); err != nil {
		return res, err
	}
	if b.file {
		if err := b.unchangedOnDisk(); err != nil {
			return res, err
		}
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
	prev := b.src.Clone()
	res.Line = b.src.Insert(txn, bean.FormatOptions{})

	// The text already had whatever problems it had; only a ledger that cannot
	// be processed at all is a reason to refuse the edit.
	led, err := b.src.Ledger()
	if led == nil {
		b.src = prev
		return res, err
	}

	if b.file {
		if err := b.writeFile(); err != nil {
			b.src = prev
			return res, err
		}
		res.Saved = true
	}
	b.led = led
	res.Invalidated = newFailures(before, b.assertionFailures())
	return res, nil
}

// unchangedOnDisk reports an error when the file's modification time is not
// the one this Book last read or wrote.
func (b *Book) unchangedOnDisk() error {
	info, err := os.Stat(b.Path)
	if err != nil {
		return err
	}
	if !b.modTime.IsZero() && !info.ModTime().Equal(b.modTime) {
		return errors.New("the ledger file changed on disk since it was opened; reopen it before adding to it")
	}
	return nil
}

// writeFile replaces the ledger file with the current text atomically: the
// new text goes to a temporary file beside it, which is then renamed over the
// original, so a crash between the two leaves either the old file or the new
// one, never a truncated one.
func (b *Book) writeFile() error {
	dir := filepath.Dir(b.Path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(b.Path)+".*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(b.src.Bytes()); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if info, err := os.Stat(b.Path); err == nil {
		_ = os.Chmod(tmp.Name(), info.Mode().Perm()) // keep the user's permissions
	}
	if err := os.Rename(tmp.Name(), b.Path); err != nil {
		cleanup()
		return err
	}
	if info, err := os.Stat(b.Path); err == nil {
		b.modTime = info.ModTime()
	}
	return nil
}

// CanEdit reports whether this ledger can be added to at all (an embedded demo
// ledger can be edited in memory but never saved).
func (b *Book) CanEdit() bool { return b.src != nil }

// writable reports whether the ledger came from a real file we can write back to.
func (b *Book) writable() bool { return b.file }

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
