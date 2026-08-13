// Package book wraps the beango accounting engine: it loads a beancount ledger
// (from a file or embedded bytes), processes it, and exposes the accounting
// views Tally renders — starting with the account balance tree.
//
// beango does the real work (parsing, validation, transaction balancing,
// cost-basis booking, balance computation); this package is the thin, app-shaped
// seam over it so the UI never touches beango's AST directly.
package book

import (
	"context"

	"github.com/dougfritz/beango/ast"
	"github.com/dougfritz/beango/ledger"
	"github.com/dougfritz/beango/loader"
)

// Book is a loaded, processed beancount ledger — the accounting model Tally draws.
type Book struct {
	// Path is the file (or logical name, for embedded ledgers) this was loaded from.
	Path string

	tree *ast.AST
	led  *ledger.Ledger

	// ProcessErr holds any error from processing (validation failures, unbalanced
	// transactions, failed balance assertions). The ledger stays usable for display
	// when this is non-nil — a beancount file with a bad assertion should still
	// open and show its data — so Tally surfaces it rather than refusing to load.
	ProcessErr error
}

// Open loads and processes the beancount file at path, following `include`s.
func Open(path string) (*Book, error) {
	res, err := loader.New(loader.WithFollowIncludes()).Load(context.Background(), path)
	if err != nil {
		return nil, err
	}
	return process(path, res.AST), nil
}

// OpenBytes loads a beancount ledger from in-memory bytes, tagged with a logical
// name for diagnostics. Used for the bundled demo ledger (go:embed) and, later,
// content handed over by a platform file picker where there is no stable path.
func OpenBytes(name string, data []byte) (*Book, error) {
	tree, err := loader.New().LoadBytes(context.Background(), name, data)
	if err != nil {
		return nil, err
	}
	return process(name, tree), nil
}

func process(path string, tree *ast.AST) *Book {
	l := ledger.New()
	perr := l.Process(context.Background(), tree)
	return &Book{Path: path, tree: tree, led: l, ProcessErr: perr}
}

// statementTypes is every beancount account type, in financial-statement order
// (balance sheet: assets, liabilities, equity; income statement: income, expenses).
var statementTypes = []ast.AccountType{
	ast.AccountTypeAssets,
	ast.AccountTypeLiabilities,
	ast.AccountTypeEquity,
	ast.AccountTypeIncome,
	ast.AccountTypeExpenses,
}

// MainCurrency reports the ledger's dominant currency: the one appearing on the
// most postings. Charts and totals are per-currency, so the UI needs one to lead
// with. (A ledger that declares `option "operating_currency"` should arguably win
// here; reading options is a later refinement.)
func (b *Book) MainCurrency() string {
	counts := map[string]int{}
	for _, acct := range b.led.Accounts() {
		for _, ap := range acct.Postings {
			if _, cur, ok := postingAmount(ap.Posting); ok {
				counts[cur]++
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
// inventory state as of the latest entry, aggregated hierarchically by account.
func (b *Book) Tree() (*ledger.BalanceTree, error) {
	return b.led.GetBalanceTree(statementTypes, nil, nil)
}
