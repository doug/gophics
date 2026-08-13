package book

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/tally/bean"
)

func testBook(t *testing.T) *Book {
	t.Helper()
	b, err := Open("../testdata/example.beancount")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return b
}

func TestSpan(t *testing.T) {
	b := testBook(t)
	first, last, ok := b.Span()
	if !ok {
		t.Fatal("no span for a ledger full of transactions")
	}
	if !first.Before(last) {
		t.Errorf("span is not ordered: %s..%s", first, last)
	}
	if first.Year() < 2000 || last.Year() > 2100 {
		t.Errorf("implausible span %s..%s", first, last)
	}
}

// TestNetWorthIsCumulative is the property that separates a net-worth line from a
// monthly-change line: each point must be a point-in-time balance of everything up
// to that date, and the final point must equal the ledger's current assets +
// liabilities.
func TestNetWorthIsCumulative(t *testing.T) {
	b := testBook(t)
	pts := b.NetWorth("USD")
	if len(pts) < 12 {
		t.Fatalf("expected a monthly series over several years, got %d points", len(pts))
	}
	for i := 1; i < len(pts); i++ {
		if !pts[i].Date.After(pts[i-1].Date) {
			t.Fatalf("points not in ascending date order at %d: %s then %s",
				i, pts[i-1].Date, pts[i].Date)
		}
	}

	// The last point must match the current balance sheet *valued at market
	// prices*, not the cash-only total.
	d := bean.NewDate(pts[len(pts)-1].Date)
	tree := b.led.BalanceTree([]string{"Assets", "Liabilities"}, bean.AsOf(d))
	want, _ := b.led.Value(tree.Flatten(), "USD", d)
	got := pts[len(pts)-1].Value
	if !got.Equal(want) {
		t.Errorf("final net worth %s != valued balance sheet %s", got, want)
	}
	if got.IsZero() {
		t.Error("final net worth is zero — the series is probably not cumulative")
	}

	// Holdings must actually be counted. This ledger buys index funds with its
	// salary, so a cash-only total is a small fraction of real net worth; if
	// conversion regressed, the two would coincide and the chart would show the
	// owner losing money while saving.
	cashOnly := tree.Flatten().Get("USD")
	if got.LessThanOrEqual(cashOnly.Mul(decimal.NewFromInt(2))) {
		t.Errorf("net worth %s barely exceeds the cash-only total %s — commodity holdings are not being valued",
			got, cashOnly)
	}
}

// TestMissingPricesNamesUnvaluedCommodities checks the honesty hook: whatever we
// cannot convert is reported rather than silently dropped from the total.
func TestMissingPricesNamesUnvaluedCommodities(t *testing.T) {
	b := testBook(t)
	missing := b.MissingPrices("USD")
	// The example ledger prices its funds, but holds vacation hours (VACHR) and
	// pre-tax 401k units (IRAUSD) that have no price — exactly the case the UI
	// needs to disclose.
	for _, cur := range missing {
		if cur == "USD" {
			t.Error("the base currency should never be reported as unpriced")
		}
		if _, ok := b.led.Price(bean.NewDate(mustLast(t, b)), cur, "USD"); ok {
			t.Errorf("%s was reported missing but has a price", cur)
		}
	}
}

func mustLast(t *testing.T, b *Book) time.Time {
	t.Helper()
	_, last, ok := b.Span()
	if !ok {
		t.Fatal("no span")
	}
	return last
}

// TestMonthlyFlowIsPeriodChange checks the counterpart property: flows are
// per-month deltas, so their sum over the ledger equals the whole-span total, and
// income comes back positive despite beancount's negative sign convention.
func TestMonthlyFlowIsPeriodChange(t *testing.T) {
	b := testBook(t)
	first, last, _ := b.Span()

	for _, tc := range []struct {
		name string
		typ  string
		neg  bool // whole-span total is negated to compare
	}{
		{"expenses", "Expenses", false},
		{"income", "Income", true},
	} {
		pts := b.MonthlyFlow(tc.typ, "USD")
		if len(pts) == 0 {
			t.Fatalf("%s: no points", tc.name)
		}
		sum := pts[0].Value
		for _, p := range pts[1:] {
			sum = sum.Add(p.Value)
		}

		tree := b.led.BalanceTree([]string{tc.typ}, bean.Between(bean.NewDate(first), bean.NewDate(last)))
		want := tree.Flatten().Get("USD")
		if tc.neg {
			want = want.Neg()
		}
		if !sum.Equal(want) {
			t.Errorf("%s: monthly totals sum to %s, whole-span total is %s", tc.name, sum, want)
		}
		if sum.IsNegative() {
			t.Errorf("%s: total should read as a positive magnitude, got %s", tc.name, sum)
		}
	}
}

func TestTopCategories(t *testing.T) {
	b := testBook(t)
	cats := b.TopCategories("USD", 2, 5)
	if len(cats) == 0 {
		t.Fatal("no expense categories")
	}
	if len(cats) > 5 {
		t.Errorf("returned %d categories, cap was 5", len(cats))
	}
	for i, c := range cats {
		if c.Total.IsZero() {
			t.Errorf("category %q has a zero total", c.Name)
		}
		if i > 0 && cats[i-1].Total.LessThan(c.Total) {
			t.Errorf("categories not sorted descending: %s then %s", cats[i-1].Total, c.Total)
		}
		// Grouped at depth 2, names carry exactly one colon ("Expenses:Food").
		if got := groupAccount(c.Name, 2); got != c.Name {
			t.Errorf("category %q is not grouped at depth 2", c.Name)
		}
	}
}

func TestGroupAccount(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Expenses:Food:Groceries", "Expenses:Food"},
		{"Expenses:Food", "Expenses:Food"},
		{"Expenses", "Expenses"},
		{"A:B:C:D", "A:B"},
	}
	for _, c := range cases {
		if got := groupAccount(c.in, 2); got != c.want {
			t.Errorf("groupAccount(%q, 2) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := groupAccount("A:B:C", 0); got != "A:B:C" {
		t.Errorf("depth 0 should pass through, got %q", got)
	}
}

func TestMonthEnds(t *testing.T) {
	first := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	ends := monthEnds(first, last)
	if len(ends) != 4 {
		t.Fatalf("got %d points, want 4 (Jan, Feb, Mar month-ends + the last date)", len(ends))
	}
	want := []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-10"}
	for i, w := range want {
		if got := ends[i].Format("2006-01-02"); got != w {
			t.Errorf("point %d = %s, want %s", i, got, w)
		}
	}
	// The series must always terminate on the ledger's real last date.
	if !ends[len(ends)-1].Equal(last) {
		t.Errorf("series ends at %s, want the ledger's last date %s", ends[len(ends)-1], last)
	}
}
