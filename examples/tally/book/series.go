package book

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/beango/ast"
	"github.com/dougfritz/beango/ledger"
)

// Point is one sample of a time series: a month-end date and a value.
type Point struct {
	Date  time.Time
	Value decimal.Decimal
}

// Category is one labelled total, used for spending breakdowns.
type Category struct {
	Name  string
	Total decimal.Decimal
}

// Span reports the dates of the ledger's first and last transaction. It reports
// ok=false for a ledger with no dated entries, so callers can skip charts rather
// than plot an empty axis.
func (b *Book) Span() (first, last time.Time, ok bool) {
	for _, acct := range b.led.Accounts() {
		for _, ap := range acct.Postings {
			d := ap.Transaction.Date()
			if d == nil {
				continue
			}
			t := d.Time
			if !ok {
				first, last, ok = t, t, true
				continue
			}
			if t.Before(first) {
				first = t
			}
			if t.After(last) {
				last = t
			}
		}
	}
	return first, last, ok
}

// NetWorth returns the month-end net worth (assets + liabilities, the latter
// being negative in beancount's sign convention) valued in the given currency,
// one point per month across the ledger's span.
//
// Two things make this the real number rather than a cash balance:
//
//   - Each point is a true point-in-time balance — every posting up to that date,
//     not just that month's — so the line is cumulative rather than a monthly delta.
//   - Holdings denominated in other commodities (shares, funds, foreign currency)
//     are converted at their price *on that date*. Without this, buying shares
//     reads as losing money: the cash leaves and the asset is invisible, producing
//     a sawtooth that trends down while the owner is actually saving.
func (b *Book) NetWorth(currency string) []Point {
	first, last, ok := b.Span()
	if !ok || currency == "" {
		return nil
	}
	types := []ast.AccountType{ast.AccountTypeAssets, ast.AccountTypeLiabilities}

	var out []Point
	for _, end := range monthEnds(first, last) {
		d := dateOf(end)
		tree, err := b.led.GetBalanceTree(types, d, d) // start == end → point-in-time
		if err != nil {
			continue
		}
		out = append(out, Point{Date: end, Value: b.valueOf(tree, currency, d)})
	}
	return out
}

// valueOf totals a balance tree in one currency, converting every other commodity
// at its price on the given date (beango forward-fills from the most recent price
// and can route through intermediate currencies).
//
// A commodity with no price path contributes nothing. That is the honest
// fallback — inventing a rate would be worse — but it means a ledger with no
// price directives for a holding under-reports; MissingPrices names them so the
// UI can say so rather than quietly showing a wrong total.
func (b *Book) valueOf(tree *ledger.BalanceTree, currency string, on *ast.Date) decimal.Decimal {
	sum := decimal.Zero
	for _, cur := range tree.Currencies {
		amount := decimal.Zero
		for _, r := range tree.Roots {
			if r.Balance != nil {
				amount = amount.Add(r.Balance.Get(cur))
			}
		}
		if amount.IsZero() {
			continue
		}
		if cur == currency {
			sum = sum.Add(amount)
			continue
		}
		if rate, ok := b.led.GetPrice(on, cur, currency); ok {
			sum = sum.Add(amount.Mul(rate))
		}
	}
	return sum
}

// MissingPrices reports commodities held at the ledger's end that cannot be
// converted to currency, so a net-worth figure can be labelled honestly instead of
// silently excluding them.
func (b *Book) MissingPrices(currency string) []string {
	_, last, ok := b.Span()
	if !ok {
		return nil
	}
	d := dateOf(last)
	tree, err := b.led.GetBalanceTree(
		[]ast.AccountType{ast.AccountTypeAssets, ast.AccountTypeLiabilities}, d, d)
	if err != nil {
		return nil
	}
	var missing []string
	for _, cur := range tree.Currencies {
		if cur == currency {
			continue
		}
		amount := decimal.Zero
		for _, r := range tree.Roots {
			if r.Balance != nil {
				amount = amount.Add(r.Balance.Get(cur))
			}
		}
		if amount.IsZero() {
			continue
		}
		if _, ok := b.led.GetPrice(d, cur, currency); !ok {
			missing = append(missing, cur)
		}
	}
	sort.Strings(missing)
	return missing
}

// MonthlyFlow returns per-month totals for one account type in the given
// currency — Income or Expenses — as a period change rather than a running total.
// Income is negated so both series read as positive magnitudes: in beancount,
// income postings are negative (money leaving the income account).
func (b *Book) MonthlyFlow(t ast.AccountType, currency string) []Point {
	first, last, ok := b.Span()
	if !ok || currency == "" {
		return nil
	}
	types := []ast.AccountType{t}

	var out []Point
	for _, end := range monthEnds(first, last) {
		start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
		tree, err := b.led.GetBalanceTree(types, dateOf(start), dateOf(end))
		if err != nil {
			continue
		}
		v := totalOf(tree, currency)
		if t == ast.AccountTypeIncome {
			v = v.Neg()
		}
		out = append(out, Point{Date: end, Value: v})
	}
	return out
}

// TopCategories returns the largest expense categories over the whole ledger,
// grouped at the given depth of the account path (depth 2 groups
// "Expenses:Food:Groceries" under "Expenses:Food"), largest first, capped at n.
func (b *Book) TopCategories(currency string, depth, n int) []Category {
	first, last, ok := b.Span()
	if !ok || currency == "" {
		return nil
	}
	tree, err := b.led.GetBalanceTree(
		[]ast.AccountType{ast.AccountTypeExpenses}, dateOf(first), dateOf(last))
	if err != nil {
		return nil
	}

	// Walk to leaves and accumulate into the requested grouping depth, so a
	// category total is independent of how deeply its subtree is nested.
	totals := map[string]decimal.Decimal{}
	var walk func(*ledger.BalanceNode)
	walk = func(nd *ledger.BalanceNode) {
		if len(nd.Children) == 0 {
			if nd.Account == "" || nd.Balance == nil {
				return
			}
			key := groupAccount(nd.Account, depth)
			totals[key] = totals[key].Add(nd.Balance.Get(currency))
			return
		}
		for _, c := range nd.Children {
			walk(c)
		}
	}
	for _, r := range tree.Roots {
		walk(r)
	}

	out := make([]Category, 0, len(totals))
	for name, total := range totals {
		if total.IsZero() {
			continue
		}
		out = append(out, Category{Name: name, Total: total})
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[j].Total.Cmp(out[i].Total); c != 0 {
			return c < 0 // descending by magnitude
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// totalOf sums every root of a balance tree in one currency.
func totalOf(tree *ledger.BalanceTree, currency string) decimal.Decimal {
	sum := decimal.Zero
	for _, r := range tree.Roots {
		if r.Balance != nil {
			sum = sum.Add(r.Balance.Get(currency))
		}
	}
	return sum
}

// groupAccount trims an account path to depth components, e.g. depth 2 turns
// "Expenses:Food:Groceries" into "Expenses:Food".
func groupAccount(account string, depth int) string {
	if depth <= 0 {
		return account
	}
	count, i := 0, 0
	for ; i < len(account); i++ {
		if account[i] == ':' {
			count++
			if count == depth {
				return account[:i]
			}
		}
	}
	return account
}

// monthEnds lists the last day of each month from first through last inclusive.
func monthEnds(first, last time.Time) []time.Time {
	var out []time.Time
	y, m := first.Year(), first.Month()
	for {
		// Day 0 of the next month is the last day of this one.
		end := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
		if end.After(last) {
			// Always include a final point at the ledger's last date, so the
			// series ends on real data rather than a future month boundary.
			out = append(out, last)
			return out
		}
		out = append(out, end)
		if m == time.December {
			y, m = y+1, time.January
		} else {
			m++
		}
	}
}

func dateOf(t time.Time) *ast.Date {
	return ast.NewDateFromTime(t)
}
