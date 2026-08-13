package book

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/dougfritz/tally/bean"
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
	f, l, has := b.led.Span()
	if !has {
		return time.Time{}, time.Time{}, false
	}
	return f.Time(), l.Time(), true
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
	types := []string{"Assets", "Liabilities"}

	var out []Point
	for _, end := range monthEnds(first, last) {
		d := bean.NewDate(end)
		tree := b.led.BalanceTree(types, bean.AsOf(d))
		value, _ := b.led.Value(tree.Flatten(), currency, d)
		out = append(out, Point{Date: end, Value: value})
	}
	return out
}

// MissingPrices reports commodities held at the ledger's end that cannot be
// converted to currency, so a net-worth figure can be labelled honestly instead of
// silently excluding them.
func (b *Book) MissingPrices(currency string) []string {
	_, last, ok := b.Span()
	if !ok {
		return nil
	}
	d := bean.NewDate(last)
	tree := b.led.BalanceTree([]string{"Assets", "Liabilities"}, bean.AsOf(d))
	_, missing := b.led.Value(tree.Flatten(), currency, d)
	sort.Strings(missing)
	return missing
}

// MonthlyFlow returns per-month totals for one account type in the given
// currency — "Income" or "Expenses" — as a period change rather than a running
// total. Income is negated so both series read as positive magnitudes: in
// beancount, income postings are negative (money leaving the income account).
func (b *Book) MonthlyFlow(accountType, currency string) []Point {
	first, last, ok := b.Span()
	if !ok || currency == "" {
		return nil
	}
	types := []string{accountType}

	var out []Point
	for _, end := range monthEnds(first, last) {
		start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
		tree := b.led.BalanceTree(types, bean.Between(bean.NewDate(start), bean.NewDate(end)))
		v := tree.Flatten().Get(currency)
		if accountType == "Income" {
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
	tree := b.led.BalanceTree([]string{"Expenses"},
		bean.Between(bean.NewDate(first), bean.NewDate(last)))

	// Walk to leaves and accumulate into the requested grouping depth, so a
	// category total is independent of how deeply its subtree is nested.
	totals := map[string]decimal.Decimal{}
	var walk func(*bean.Node)
	walk = func(nd *bean.Node) {
		if len(nd.Children) == 0 {
			if nd.Account == "" {
				return
			}
			key := groupAccount(string(nd.Account), depth)
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

// groupAccount trims an account path to depth components, e.g. depth 2 turns
// "Expenses:Food:Groceries" into "Expenses:Food".
func groupAccount(account string, depth int) string {
	if depth <= 0 {
		return account
	}
	count := 0
	for i := 0; i < len(account); i++ {
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
