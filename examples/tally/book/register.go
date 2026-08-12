package book

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/beango/ast"
	"github.com/dougfritz/beango/ledger"
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
	acct, ok := b.led.GetAccount(account)
	if !ok {
		return nil, nil
	}
	if currency == "" {
		currency = dominantCurrency(acct)
	}

	entries := make([]Entry, 0, len(acct.Postings))
	running := decimal.Zero
	for _, ap := range acct.Postings {
		amt, cur, ok := postingAmount(ap.Posting)
		if !ok || cur != currency {
			continue
		}
		running = running.Add(amt)
		txn := ap.Transaction
		entries = append(entries, Entry{
			Date:      dateString(txn),
			Payee:     txn.Payee.String(),
			Narration: txn.Narration.String(),
			Other:     counterparts(txn, account),
			Amount:    amt,
			Currency:  cur,
			Balance:   running,
			Flag:      txn.Flag,
		})
	}
	return entries, nil
}

// Currencies lists the currencies an account has postings in, most-used first,
// so the UI can offer a picker for multi-commodity accounts.
func (b *Book) Currencies(account string) []string {
	acct, ok := b.led.GetAccount(account)
	if !ok {
		return nil
	}
	return currenciesByUse(acct)
}

// AccountNames returns every account name in the ledger, sorted.
func (b *Book) AccountNames() []string {
	accts := b.led.Accounts()
	names := make([]string, 0, len(accts))
	for name := range accts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// postingAmount reads a posting's amount as a decimal. Postings whose amount was
// elided in the source are filled in by the ledger's interpolation, but a posting
// with no resolvable amount (or an unparsable one) is skipped by the caller.
func postingAmount(p *ast.Posting) (decimal.Decimal, string, bool) {
	if p == nil || p.Amount == nil {
		return decimal.Zero, "", false
	}
	d, err := decimal.NewFromString(p.Amount.Value)
	if err != nil {
		return decimal.Zero, "", false
	}
	return d, p.Amount.Currency, true
}

// counterparts names the other accounts in the transaction, leaf-first and
// de-duplicated: the "where did this go" column of a register.
func counterparts(txn *ast.Transaction, self string) string {
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

func dateString(txn *ast.Transaction) string {
	if d := txn.Date(); d != nil {
		return d.String()
	}
	return ""
}

// currenciesByUse orders an account's currencies by posting count, descending.
func currenciesByUse(acct *ledger.Account) []string {
	counts := map[string]int{}
	for _, ap := range acct.Postings {
		if _, cur, ok := postingAmount(ap.Posting); ok {
			counts[cur]++
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

func dominantCurrency(acct *ledger.Account) string {
	if curs := currenciesByUse(acct); len(curs) > 0 {
		return curs[0]
	}
	return ""
}
