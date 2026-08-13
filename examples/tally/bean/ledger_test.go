package bean

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// TestWeightUsesCostThenPrice pins the rule that makes cross-commodity
// transactions balance: a posting weighs what it cost, not what it holds.
func TestWeightUsesCostThenPrice(t *testing.T) {
	f := parseString(t, `2021-05-01 * "Mixed"
  Assets:GLD      10 GLD {180.00 USD}
  Assets:ITOT      5 ITOT {{500.00 USD}}
  Assets:SoldGLD  -2 GLD {{300.00 USD}}
  Assets:Cash   -100.00 USD @ 1.25 CAD
  Assets:Other  -200.00 USD @@ 250.00 CAD
  Assets:Plain    42.00 USD
`)
	txn := f.Directives[0].(*Transaction)
	want := []Amount{
		{Number: dec("1800"), Currency: "USD"}, // 10 × 180
		{Number: dec("500"), Currency: "USD"},  // total cost as written
		{Number: dec("-300"), Currency: "USD"}, // total cost follows the units' sign
		{Number: dec("-125"), Currency: "CAD"}, // -100 × 1.25
		{Number: dec("-250"), Currency: "CAD"}, // total price, sign from units
		{Number: dec("42"), Currency: "USD"},   // no cost or price: itself
	}
	for i, w := range want {
		got, ok := txn.Postings[i].Weight()
		if !ok {
			t.Fatalf("posting %d has no weight", i)
		}
		if !got.Number.Equal(w.Number) || got.Currency != w.Currency {
			t.Errorf("posting %d weight = %s, want %s", i, got, w)
		}
	}
}

func TestBalanceInfersElidedAmount(t *testing.T) {
	f := parseString(t, `2021-01-04 * "Rent"
  Expenses:Home:Rent  2400.00 USD
  Assets:Checking
`)
	txn := f.Directives[0].(*Transaction)
	if err := txn.Balance(); err != nil {
		t.Fatalf("Balance: %v", err)
	}
	p := txn.Postings[1]
	if p.Amount == nil {
		t.Fatal("elided amount was not inferred")
	}
	if !p.Amount.Number.Equal(dec("-2400")) || p.Amount.Currency != "USD" {
		t.Errorf("inferred %s, want -2400 USD", p.Amount)
	}
	if !p.Inferred {
		t.Error("inferred posting is not marked Inferred")
	}
}

// TestBalanceInfersAcrossCost is the case that matters for investing: buying
// shares elides the cash leg, and the inferred amount must come from the cost, not
// the share count.
func TestBalanceInfersAcrossCost(t *testing.T) {
	f := parseString(t, `2021-05-01 * "Buy"
  Assets:ETrade:GLD   10 GLD {180.00 USD}
  Assets:ETrade:Cash
`)
	txn := f.Directives[0].(*Transaction)
	if err := txn.Balance(); err != nil {
		t.Fatalf("Balance: %v", err)
	}
	got := txn.Postings[1].Amount
	if got == nil || !got.Number.Equal(dec("-1800")) || got.Currency != "USD" {
		t.Errorf("inferred %v, want -1800 USD", got)
	}
}

func TestBalanceRejectsUnbalanced(t *testing.T) {
	f := parseString(t, `2021-01-04 * "Wrong"
  Assets:Checking  -100.00 USD
  Expenses:Food      90.00 USD
`)
	err := f.Directives[0].(*Transaction).Balance()
	if err == nil {
		t.Fatal("expected an imbalance to be reported")
	}
	var be *BalanceError
	if !asBalanceError(err, &be) {
		t.Fatalf("got %T, want *BalanceError", err)
	}
}

// TestBalanceTolerance checks that ordinary rounding passes but real errors do
// not: amounts written to the cent tolerate half a cent.
func TestBalanceTolerance(t *testing.T) {
	within := parseString(t, `2021-01-04 * "Rounding"
  Assets:A  -10.00 USD
  Expenses:B  3.33 USD
  Expenses:C  3.33 USD
  Expenses:D  3.34 USD
`)
	if err := within.Directives[0].(*Transaction).Balance(); err != nil {
		t.Errorf("a transaction that sums exactly should balance: %v", err)
	}

	off := parseString(t, `2021-01-04 * "Off by a cent"
  Assets:A  -10.00 USD
  Expenses:B  3.33 USD
  Expenses:C  3.33 USD
  Expenses:D  3.33 USD
`)
	if err := off.Directives[0].(*Transaction).Balance(); err == nil {
		t.Error("a whole cent out should not be tolerated")
	}
}

func TestBalanceRejectsTwoElided(t *testing.T) {
	f := parseString(t, `2021-01-04 * "Ambiguous"
  Assets:A  -10.00 USD
  Expenses:B
  Expenses:C
`)
	if err := f.Directives[0].(*Transaction).Balance(); err == nil {
		t.Error("two elided amounts cannot be inferred and must be reported")
	}
}

func TestProcessAccountsAndBalances(t *testing.T) {
	l, err := LoadString("t.beancount", `
2021-01-01 open Assets:Checking USD
2021-01-01 open Expenses:Food USD
2021-01-05 * "Groceries"
  Assets:Checking  -50.00 USD
  Expenses:Food     50.00 USD
2021-02-05 * "More groceries"
  Assets:Checking  -25.00 USD
  Expenses:Food     25.00 USD
`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}
	if len(l.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", l.Problems)
	}
	if got := l.BalanceOf("Assets:Checking").Get("USD"); !got.Equal(dec("-75")) {
		t.Errorf("checking = %s, want -75", got)
	}
	if got := l.BalanceOf("Expenses:Food").Get("USD"); !got.Equal(dec("75")) {
		t.Errorf("food = %s, want 75", got)
	}

	// Point-in-time and period readings must differ appropriately.
	asOfJan := l.BalanceAsOf("Assets:Checking", Date{2021, 1, 31}).Get("USD")
	if !asOfJan.Equal(dec("-50")) {
		t.Errorf("as of 2021-01-31 = %s, want -50", asOfJan)
	}
	feb := l.BalanceBetween("Expenses:Food", Date{2021, 2, 1}, Date{2021, 2, 28}).Get("USD")
	if !feb.Equal(dec("25")) {
		t.Errorf("February food = %s, want 25", feb)
	}
}

func TestAssertionPassesAndFails(t *testing.T) {
	l, _ := LoadString("t.beancount", `
2021-01-01 open Assets:Checking USD
2021-01-05 * "Deposit"
  Assets:Checking   100.00 USD
  Income:Salary    -100.00 USD
2021-01-06 balance Assets:Checking  100.00 USD
`)
	if len(l.Problems) != 0 {
		t.Errorf("a correct assertion should not be a problem: %v", l.Problems)
	}

	bad, _ := LoadString("t.beancount", `
2021-01-05 * "Deposit"
  Assets:Checking   100.00 USD
  Income:Salary    -100.00 USD
2021-01-06 balance Assets:Checking  999.00 USD
`)
	if len(bad.Problems) != 1 {
		t.Fatalf("expected one failed assertion, got %v", bad.Problems)
	}
	if _, ok := bad.Problems[0].(*AssertionError); !ok {
		t.Errorf("got %T, want *AssertionError", bad.Problems[0])
	}
}

// TestPadSatisfiesAssertion checks that a pad inserts exactly the difference, so
// the assertion that follows it holds.
func TestPadSatisfiesAssertion(t *testing.T) {
	l, _ := LoadString("t.beancount", `
2021-01-01 open Assets:Checking USD
2021-01-01 open Equity:Opening-Balances USD
2021-01-01 pad Assets:Checking Equity:Opening-Balances
2021-01-02 balance Assets:Checking  500.00 USD
`)
	if len(l.Problems) != 0 {
		t.Fatalf("pad should have satisfied the assertion: %v", l.Problems)
	}
	if got := l.BalanceOf("Assets:Checking").Get("USD"); !got.Equal(dec("500")) {
		t.Errorf("checking after pad = %s, want 500", got)
	}
	if got := l.BalanceOf("Equity:Opening-Balances").Get("USD"); !got.Equal(dec("-500")) {
		t.Errorf("equity after pad = %s, want -500", got)
	}
}

func TestPriceForwardFillAndRouting(t *testing.T) {
	l, _ := LoadString("t.beancount", `
2021-01-01 price GLD  180.00 USD
2021-03-01 price GLD  200.00 USD
2021-01-01 price EUR  1.10 USD
`)
	// Exact date, and a later date forward-filling from the most recent quote.
	if r, ok := l.Price(Date{2021, 1, 1}, "GLD", "USD"); !ok || !r.Equal(dec("180")) {
		t.Errorf("GLD on 2021-01-01 = %s (%v), want 180", r, ok)
	}
	if r, ok := l.Price(Date{2021, 2, 15}, "GLD", "USD"); !ok || !r.Equal(dec("180")) {
		t.Errorf("GLD on 2021-02-15 should forward-fill 180, got %s", r)
	}
	if r, ok := l.Price(Date{2021, 6, 1}, "GLD", "USD"); !ok || !r.Equal(dec("200")) {
		t.Errorf("GLD on 2021-06-01 = %s, want 200", r)
	}
	// Before any quote there is no rate to fill from.
	if _, ok := l.Price(Date{2020, 1, 1}, "GLD", "USD"); ok {
		t.Error("a date before the first quote should have no price")
	}
	// The inverse comes for free.
	if r, ok := l.Price(Date{2021, 1, 1}, "USD", "EUR"); !ok || !r.Round(4).Equal(dec("0.9091")) {
		t.Errorf("USD→EUR = %s, want ~0.9091", r)
	}
	// And a route through an intermediate: GLD→USD→EUR.
	r, ok := l.Price(Date{2021, 1, 1}, "GLD", "EUR")
	if !ok {
		t.Fatal("no route from GLD to EUR")
	}
	if !r.Round(4).Equal(dec("163.6364")) {
		t.Errorf("GLD→EUR = %s, want ~163.6364 (180/1.10)", r)
	}
	if r, _ := l.Price(Date{2021, 1, 1}, "USD", "USD"); !r.Equal(dec("1")) {
		t.Errorf("identity conversion = %s, want 1", r)
	}
}

func TestValueReportsMissingPrices(t *testing.T) {
	l, _ := LoadString("t.beancount", `2021-01-01 price GLD  180.00 USD`)
	b := NewBalance()
	b.Add("USD", dec("100"))
	b.Add("GLD", dec("2"))
	b.Add("VACHR", dec("40")) // vacation hours: real holding, no price

	total, missing := l.Value(b, "USD", Date{2021, 6, 1})
	if !total.Equal(dec("460")) { // 100 + 2×180
		t.Errorf("value = %s, want 460", total)
	}
	if len(missing) != 1 || missing[0] != "VACHR" {
		t.Errorf("missing = %v, want [VACHR]", missing)
	}
}

func TestBalanceTreeAggregates(t *testing.T) {
	l, _ := LoadString("t.beancount", `
2021-01-05 * "Groceries"
  Assets:US:BofA:Checking  -50.00 USD
  Expenses:Food:Groceries    50.00 USD
2021-01-06 * "Restaurant"
  Assets:US:BofA:Checking  -30.00 USD
  Expenses:Food:Dining       30.00 USD
`)
	tree := l.BalanceTree([]string{"Expenses"}, All())
	if len(tree.Roots) != 1 || tree.Roots[0].Name != "Expenses" {
		t.Fatalf("roots = %+v", tree.Roots)
	}
	// The root totals its subtree.
	if got := tree.Roots[0].Balance.Get("USD"); !got.Equal(dec("80")) {
		t.Errorf("Expenses total = %s, want 80", got)
	}
	// Intermediate nodes are created even though nothing posts to them directly.
	food := tree.Roots[0].Children[0]
	if food.Account != "Expenses:Food" || !food.Balance.Get("USD").Equal(dec("80")) {
		t.Errorf("Expenses:Food = %s %s", food.Account, food.Balance)
	}
	if len(food.Children) != 2 {
		t.Fatalf("expected two leaves under Expenses:Food, got %d", len(food.Children))
	}
	// Children are sorted for stable display.
	if food.Children[0].Name != "Dining" || food.Children[1].Name != "Groceries" {
		t.Errorf("children not sorted: %s, %s", food.Children[0].Name, food.Children[1].Name)
	}
	// The type filter excludes Assets entirely.
	l.BalanceTree([]string{"Expenses"}, All()).Walk(func(n *Node) {
		if n.Account.Type() == "Assets" {
			t.Errorf("Assets leaked into an Expenses-only tree: %s", n.Account)
		}
	})
}

func TestBalanceTreePointInTimeVsPeriod(t *testing.T) {
	l, _ := LoadString("t.beancount", `
2021-01-05 * "January"
  Assets:Checking  -50.00 USD
  Expenses:Food     50.00 USD
2021-02-05 * "February"
  Assets:Checking  -25.00 USD
  Expenses:Food     25.00 USD
`)
	cumulative := l.BalanceTree([]string{"Expenses"}, AsOf(Date{2021, 2, 28}))
	if got := cumulative.Flatten().Get("USD"); !got.Equal(dec("75")) {
		t.Errorf("cumulative through February = %s, want 75", got)
	}
	period := l.BalanceTree([]string{"Expenses"}, Between(Date{2021, 2, 1}, Date{2021, 2, 28}))
	if got := period.Flatten().Get("USD"); !got.Equal(dec("25")) {
		t.Errorf("February alone = %s, want 25", got)
	}
}

func asBalanceError(err error, out **BalanceError) bool {
	be, ok := err.(*BalanceError)
	if ok {
		*out = be
	}
	return ok
}

// TestRealLedgerSelfVerifies is the strongest correctness check available without
// a second implementation: a real ledger asserts its own balances, so processing
// it cleanly means the engine's arithmetic agrees with what the author declared.
// The sample carries 78 balance assertions across 1,007 transactions — including
// fractional-share purchases whose cost-scaled rounding is exactly where a naive
// tolerance rule goes wrong.
func TestRealLedgerSelfVerifies(t *testing.T) {
	l, err := Load("../demo/example.beancount")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(l.Problems) != 0 {
		for i, p := range l.Problems {
			if i < 10 {
				t.Errorf("problem: %v", p)
			}
		}
		t.Fatalf("%d problems processing a ledger that should verify cleanly", len(l.Problems))
	}

	assertions := 0
	for _, d := range l.Directives {
		if a, ok := d.(*Assertion); ok {
			assertions++
			// Re-check each assertion directly: the balance strictly before its
			// date must equal what it claims.
			got := l.balanceBefore(a.Account, a.Date).Get(a.Amount.Currency)
			if !got.Equal(a.Amount.Number) {
				t.Errorf("%s: %s computed %s, ledger asserts %s",
					a.Where(), a.Account, got, a.Amount)
			}
		}
	}
	if assertions != 78 {
		t.Errorf("checked %d assertions, expected 78", assertions)
	}

	// And the accounts came out whole.
	if n := len(l.Accounts()); n != 63 {
		t.Errorf("%d accounts, want 63", n)
	}
}
