package book

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/beango/ledger"
)

// findNodeBalance walks a balance tree for the node of the named account and
// returns its balance in the given currency.
func findNodeBalance(tr *ledger.BalanceTree, account, currency string) (decimal.Decimal, bool) {
	var walk func(n *ledger.BalanceNode) (decimal.Decimal, bool)
	walk = func(n *ledger.BalanceNode) (decimal.Decimal, bool) {
		if n.Account == account && n.Balance != nil {
			return n.Balance.Get(currency), true
		}
		for _, c := range n.Children {
			if d, ok := walk(c); ok {
				return d, true
			}
		}
		return decimal.Zero, false
	}
	for _, r := range tr.Roots {
		if d, ok := walk(r); ok {
			return d, true
		}
	}
	return decimal.Zero, false
}

// TestRegisterRunningBalance checks a real account's register: postings come back
// chronologically, in one currency, and the running balance is the accumulation
// of the amounts.
func TestRegisterRunningBalance(t *testing.T) {
	b, err := Open("../testdata/example.beancount")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Pick a checking account from the example ledger — it has many USD postings.
	var target string
	for _, name := range b.AccountNames() {
		if name == "Assets:US:BofA:Checking" {
			target = name
			break
		}
	}
	if target == "" {
		t.Skip("example ledger has no Assets:US:BofA:Checking account")
	}

	entries, err := b.Register(target, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no register entries for %s", target)
	}

	sum := entries[0].Amount
	for i, e := range entries {
		if e.Date == "" {
			t.Errorf("entry %d has no date", i)
		}
		if e.Currency != entries[0].Currency {
			t.Errorf("entry %d currency %q != %q — register must be single-currency", i, e.Currency, entries[0].Currency)
		}
		if i > 0 {
			if e.Date < entries[i-1].Date {
				t.Errorf("entry %d date %s precedes previous %s — not chronological", i, e.Date, entries[i-1].Date)
			}
			sum = sum.Add(e.Amount)
		}
		if !e.Balance.Equal(sum) {
			t.Fatalf("entry %d running balance %s != accumulated %s", i, e.Balance, sum)
		}
	}

	// The final running balance must match the account's balance in the tree.
	tr, err := b.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	last := entries[len(entries)-1]
	if got, ok := findNodeBalance(tr, target, last.Currency); ok && !got.Equal(last.Balance) {
		t.Errorf("register final balance %s != balance tree %s for %s", last.Balance, got, target)
	}
}

// TestCurrenciesForAccount pins down how commodities map to accounts in
// beancount: each commodity gets its own leaf account (ETrade:GLD holds GLD,
// ETrade:Cash holds USD), so a posting-bearing leaf is single-currency. The
// multi-commodity totals seen in the UI come from the balance *tree* aggregating
// children — parent accounts like Assets:US:ETrade carry no postings themselves.
func TestCurrenciesForAccount(t *testing.T) {
	b, err := Open("../testdata/example.beancount")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	leaves := 0
	for _, name := range b.AccountNames() {
		curs := b.Currencies(name)
		if len(curs) == 0 {
			continue // a parent/aggregate account, or one with no postings
		}
		leaves++
		if len(curs) > 1 {
			// Not an error in principle, but the register's running balance is
			// per-currency; make sure Register still restricts to one series.
			entries, err := b.Register(name, curs[0])
			if err != nil {
				t.Fatalf("Register(%s): %v", name, err)
			}
			for _, e := range entries {
				if e.Currency != curs[0] {
					t.Fatalf("%s: register mixed %q into a %q series", name, e.Currency, curs[0])
				}
			}
		}
	}
	if leaves == 0 {
		t.Fatal("no posting-bearing accounts found in the example ledger")
	}

	// A known aggregate parent has children but no postings of its own.
	if curs := b.Currencies("Assets:US:ETrade"); len(curs) != 0 {
		t.Errorf("Assets:US:ETrade is an aggregate parent; expected no own postings, got %v", curs)
	}
	// And a known commodity leaf reports exactly its commodity.
	if curs := b.Currencies("Assets:US:ETrade:GLD"); len(curs) != 1 || curs[0] != "GLD" {
		t.Errorf("Assets:US:ETrade:GLD currencies = %v, want [GLD]", curs)
	}
}
