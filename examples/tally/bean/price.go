package bean

import (
	"sort"

	"github.com/doug/tally/decimal"
)

// pricePoint is one dated rate for a commodity pair.
type pricePoint struct {
	date Date
	rate decimal.Decimal
}

// pair identifies a directed conversion.
type pair struct{ from, to string }

// priceGraph holds every price directive, indexed by pair and sorted by date.
//
// Two behaviours make it useful for valuing holdings. Prices are *forward-filled*:
// asking for a rate on a date returns the most recent one on or before it, because
// a commodity does not become unpriced between quotes. And conversions may route
// through intermediates, so a fund priced in EUR can still be valued in USD when
// only EUR/USD is quoted.
type priceGraph struct {
	points map[pair][]pricePoint
	sorted bool
}

func newPriceGraph() *priceGraph {
	return &priceGraph{points: map[pair][]pricePoint{}}
}

// add records that one unit of from was worth rate units of to on a date, and the
// inverse, so a rate quoted in one direction is usable in both.
func (g *priceGraph) add(d Date, from, to string, rate decimal.Decimal) {
	if from == "" || to == "" || rate.IsZero() {
		return
	}
	g.points[pair{from, to}] = append(g.points[pair{from, to}], pricePoint{d, rate})
	inv := decimal.NewFromInt(1).Div(rate)
	g.points[pair{to, from}] = append(g.points[pair{to, from}], pricePoint{d, inv})
	g.sorted = false
}

// ensureSorted orders each pair's points by date, once, on first lookup.
func (g *priceGraph) ensureSorted() {
	if g.sorted {
		return
	}
	for k := range g.points {
		pts := g.points[k]
		sort.Slice(pts, func(i, j int) bool { return pts[i].date.Before(pts[j].date) })
		g.points[k] = pts
	}
	g.sorted = true
}

// direct returns the most recent rate for a pair on or before a date.
func (g *priceGraph) direct(d Date, from, to string) (decimal.Decimal, bool) {
	g.ensureSorted()
	pts := g.points[pair{from, to}]
	// Points are ascending, so walk back from the end to the first one that is
	// not in the future.
	for i := len(pts) - 1; i >= 0; i-- {
		if !pts[i].date.After(d) {
			return pts[i].rate, true
		}
	}
	return decimal.Zero, false
}

// Price converts one unit of from into to, as of a date.
//
// It tries the direct rate first, then searches for a route through intermediate
// commodities (breadth-first, so the answer uses the fewest conversions and the
// least compounding error). Rates along the route are forward-filled individually.
func (g *priceGraph) Price(d Date, from, to string) (decimal.Decimal, bool) {
	if from == to {
		return decimal.NewFromInt(1), true
	}
	if rate, ok := g.direct(d, from, to); ok {
		return rate, true
	}
	return g.route(d, from, to)
}

// route breadth-first searches the conversion graph.
func (g *priceGraph) route(d Date, from, to string) (decimal.Decimal, bool) {
	g.ensureSorted()

	// Neighbours of a commodity: every currency it has any price against.
	neighbours := map[string][]string{}
	for p := range g.points {
		neighbours[p.from] = append(neighbours[p.from], p.to)
	}

	type step struct {
		cur  string
		rate decimal.Decimal
	}
	seen := map[string]bool{from: true}
	queue := []step{{from, decimal.NewFromInt(1)}}

	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]

		next := neighbours[s.cur]
		sort.Strings(next) // deterministic route choice between equal-length paths
		for _, n := range next {
			if seen[n] {
				continue
			}
			leg, ok := g.direct(d, s.cur, n)
			if !ok {
				continue // no rate for this leg as of the date
			}
			rate := s.rate.Mul(leg)
			if n == to {
				return rate, true
			}
			seen[n] = true
			queue = append(queue, step{n, rate})
		}
	}
	return decimal.Zero, false
}

// Price converts one unit of from into to as of a date, forward-filling and
// routing through intermediates as needed.
func (l *Ledger) Price(d Date, from, to string) (decimal.Decimal, bool) {
	return l.prices.Price(d, from, to)
}

// Value converts a whole balance into one commodity as of a date.
//
// Anything that cannot be converted is reported in missing rather than silently
// dropped: a net-worth figure that quietly omits an unpriced holding is worse than
// one that says what it left out.
func (l *Ledger) Value(b Balance, to string, d Date) (total decimal.Decimal, missing []string) {
	for _, cur := range b.Currencies() {
		units := b[cur]
		if units.IsZero() {
			continue
		}
		rate, ok := l.Price(d, cur, to)
		if !ok {
			missing = append(missing, cur)
			continue
		}
		total = total.Add(units.Mul(rate))
	}
	return total, missing
}
