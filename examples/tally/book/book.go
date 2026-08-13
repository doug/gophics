// Package book wraps the accounting engine: it loads a beancount ledger (from a
// file or embedded bytes), processes it, and exposes the views Tally renders.
//
// The engine is bean, this repository's Apache-2.0 implementation, so nothing
// GPL-licensed is linked into a shipped build. This package stays a thin,
// app-shaped seam over it so the UI never touches the AST directly — which is
// what made swapping the engine underneath a contained change.
package book

import (
	"github.com/dougfritz/tally/bean"
)

// Book is a loaded, processed beancount ledger — the accounting model Tally draws.
type Book struct {
	// Path is the file (or logical name, for embedded ledgers) this was loaded from.
	Path string

	led *bean.Ledger

	// ProcessErr holds any error from loading (a missing include, a syntax error).
	// The ledger stays usable for display when this is non-nil — a file with one
	// bad line should still open and show its data — so Tally surfaces it rather
	// than refusing to load.
	ProcessErr error
}

// Open loads and processes the beancount file at path, following `include`s.
func Open(path string) (*Book, error) {
	led, err := bean.Load(path)
	if led == nil {
		return nil, err
	}
	return &Book{Path: path, led: led, ProcessErr: err}, nil
}

// OpenBytes loads a beancount ledger from in-memory bytes, tagged with a logical
// name for diagnostics. Used for the bundled demo ledger (go:embed) and for
// content handed over by a platform file picker where there is no stable path.
func OpenBytes(name string, data []byte) (*Book, error) {
	led, err := bean.LoadString(name, string(data))
	if led == nil {
		return nil, err
	}
	return &Book{Path: name, led: led, ProcessErr: err}, nil
}

// Problems reports the semantic issues found while processing — unbalanced
// transactions, failed balance assertions — so the UI can surface them.
func (b *Book) Problems() []error { return b.led.Problems }

// statementTypes is every beancount account type, in financial-statement order
// (balance sheet: assets, liabilities, equity; income statement: income, expenses).
var statementTypes = []string{"Assets", "Liabilities", "Equity", "Income", "Expenses"}

// MainCurrency reports the ledger's dominant currency: the one appearing on the
// most postings. Charts and totals are per-currency, so the UI needs one to lead
// with. (A ledger that declares `option "operating_currency"` should arguably win
// here; reading options is a later refinement.)
func (b *Book) MainCurrency() string {
	counts := map[string]int{}
	for _, a := range b.led.Accounts() {
		for _, ref := range a.Postings {
			if ref.Posting.Amount != nil {
				counts[ref.Posting.Amount.Currency]++
			}
		}
	}
	best, bestN := "", 0
	for cur, n := range counts {
		// Ties break alphabetically so the choice is stable across runs.
		if n > bestN || (n == bestN && cur < best) {
			best, bestN = cur, n
		}
	}
	return best
}

// Tree returns the current balance tree across all account types: the running
// state as of the latest entry, aggregated hierarchically by account.
func (b *Book) Tree() (*bean.Tree, error) {
	return b.led.BalanceTree(statementTypes, bean.All()), nil
}
