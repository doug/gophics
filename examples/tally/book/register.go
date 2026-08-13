package book

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/tally/bean"
)

// Entry is one row of an account register: a posting on that account, with the
// transaction context a reader needs and the account balance after it.
type Entry struct {
	Date      string // ISO date of the parent transaction
	Payee     string
	Narration string
	// Other names the counterpart account(s) — where the money came from or went
	// — which is what makes a register readable ("Groceries", "Salary").
	Other string
	// Amount is this posting's effect on the register's account, in Currency.
	Amount   decimal.Decimal
	Currency string
	// Balance is the account's running total in Currency after this posting.
	Balance decimal.Decimal
	// Flag is the transaction flag ("*" cleared, "!" pending).
	Flag string
}

// Register returns the chronological register for one account, restricted to a
// single currency (the account's dominant one when currency is empty), with a
// running balance. Restricting to one currency is what makes the running-balance
// column meaningful: a multi-commodity account (a brokerage holding shares and
// cash) has no single scalar total, so the caller picks which series to read.
func (b *Book) Register(account, currency string) ([]Entry, error) {
	acct, ok := b.led.Account(bean.Account(account))
	if !ok {
		return nil, nil
	}
	if currency == "" {
		currency = dominantCurrency(acct)
	}

	entries := make([]Entry, 0, len(acct.Postings))
	running := decimal.Zero
	for _, ref := range acct.Postings {
		amt := ref.Posting.Amount
		if amt == nil || amt.Currency != currency {
			continue
		}
		running = running.Add(amt.Number)
		txn := ref.Txn
		entries = append(entries, Entry{
			Date:      txn.When().String(),
			Payee:     txn.Payee,
			Narration: txn.Narration,
			Other:     counterparts(txn, account),
			Amount:    amt.Number,
			Currency:  amt.Currency,
			Balance:   running,
			Flag:      txn.Flag,
		})
	}
	return entries, nil
}

// Currencies lists the currencies an account has postings in, most-used first,
// so the UI can offer a picker for multi-commodity accounts.
func (b *Book) Currencies(account string) []string {
	acct, ok := b.led.Account(bean.Account(account))
	if !ok {
		return nil
	}
	return currenciesByUse(acct)
}

// AccountNames returns every account name in the ledger, sorted.
func (b *Book) AccountNames() []string {
	names := b.led.AccountNames()
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = string(n)
	}
	return out
}

// counterparts names the other accounts in the transaction, in source order and
// de-duplicated: the "where did this go" column of a register.
func counterparts(txn *bean.Transaction, self string) string {
	seen := map[string]bool{}
	parts := make([]string, 0, 2)
	for _, p := range txn.Postings {
		name := string(p.Account)
		if name == self || seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

// currenciesByUse orders an account's currencies by posting count, descending.
func currenciesByUse(acct *bean.AccountInfo) []string {
	counts := map[string]int{}
	for _, ref := range acct.Postings {
		if a := ref.Posting.Amount; a != nil {
			counts[a.Currency]++
		}
	}
	curs := make([]string, 0, len(counts))
	for c := range counts {
		curs = append(curs, c)
	}
	sort.Slice(curs, func(i, j int) bool {
		if counts[curs[i]] != counts[curs[j]] {
			return counts[curs[i]] > counts[curs[j]]
		}
		return curs[i] < curs[j] // stable for equal counts
	})
	return curs
}

func dominantCurrency(acct *bean.AccountInfo) string {
	if curs := currenciesByUse(acct); len(curs) > 0 {
		return curs[0]
	}
	return ""
}
