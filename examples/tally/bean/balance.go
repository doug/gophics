package bean

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Balance is an amount held across one or more commodities — the running total of
// an account, or the residual of a transaction. Commodities never mix: 10 GLD and
// 500 USD are two independent entries, and only a price can relate them.
type Balance map[string]decimal.Decimal

// NewBalance returns an empty balance.
func NewBalance() Balance { return Balance{} }

// Add accumulates units of a commodity, dropping the entry when it reaches zero
// so a balance that nets out compares equal to an empty one.
func (b Balance) Add(currency string, units decimal.Decimal) {
	if currency == "" {
		return
	}
	sum := b[currency].Add(units)
	if sum.IsZero() {
		delete(b, currency)
		return
	}
	b[currency] = sum
}

// AddAmount accumulates an Amount.
func (b Balance) AddAmount(a Amount) { b.Add(a.Currency, a.Number) }

// Merge accumulates every commodity of another balance.
func (b Balance) Merge(o Balance) {
	for cur, units := range o {
		b.Add(cur, units)
	}
}

// Get returns the units held in a commodity, or zero.
func (b Balance) Get(currency string) decimal.Decimal { return b[currency] }

// Currencies lists the commodities held, sorted, for deterministic output.
func (b Balance) Currencies() []string {
	out := make([]string, 0, len(b))
	for cur := range b {
		out = append(out, cur)
	}
	sort.Strings(out)
	return out
}

// IsZero reports a balance holding nothing.
func (b Balance) IsZero() bool { return len(b) == 0 }

// Clone returns an independent copy.
func (b Balance) Clone() Balance {
	out := make(Balance, len(b))
	for cur, units := range b {
		out[cur] = units
	}
	return out
}

// Neg returns the balance with every amount negated.
func (b Balance) Neg() Balance {
	out := make(Balance, len(b))
	for cur, units := range b {
		out[cur] = units.Neg()
	}
	return out
}

func (b Balance) String() string {
	curs := b.Currencies()
	parts := make([]string, len(curs))
	for i, cur := range curs {
		parts[i] = b[cur].String() + " " + cur
	}
	return strings.Join(parts, ", ")
}

// Weight is a posting's contribution to its transaction's balance.
//
// This is the rule that makes double-entry work across commodities. A posting
// normally weighs its own units in its own commodity — but when it carries a cost
// or a price, it weighs what it *cost*, in that commodity:
//
//	10 GLD {180.00 USD}          weighs  1800.00 USD
//	10 GLD {{1800.00 USD}}       weighs  1800.00 USD
//	-100.00 USD @ 1.25 CAD       weighs  -125.00 CAD
//	-100.00 USD @@ 125.00 CAD    weighs  -125.00 CAD
//
// That is what lets "buy shares with dollars" balance: both legs weigh dollars.
// Cost takes precedence over price, as in beancount — a posting with both is
// recording what it paid *and* the market rate, and the cost is what balances.
func (p *Posting) Weight() (Amount, bool) {
	if p.Amount == nil {
		return Amount{}, false
	}
	switch {
	case p.Cost != nil && p.Cost.Amount != nil:
		return Amount{
			Number:   p.Amount.Number.Mul(p.Cost.Amount.Number),
			Currency: p.Cost.Amount.Currency,
		}, true

	case p.Cost != nil && p.Cost.Total != nil:
		// A total cost already covers every unit; its sign follows the units, so
		// selling (negative units) weighs negative.
		total := p.Cost.Total.Number.Abs()
		if p.Amount.Number.IsNegative() {
			total = total.Neg()
		}
		return Amount{Number: total, Currency: p.Cost.Total.Currency}, true

	case p.Price != nil && p.PriceTotal:
		total := p.Price.Number.Abs()
		if p.Amount.Number.IsNegative() {
			total = total.Neg()
		}
		return Amount{Number: total, Currency: p.Price.Currency}, true

	case p.Price != nil:
		return Amount{
			Number:   p.Amount.Number.Mul(p.Price.Number),
			Currency: p.Price.Currency,
		}, true
	}
	return *p.Amount, true
}

// Residual returns what a transaction's postings fail to cancel out. A balanced
// transaction has an empty residual.
func (t *Transaction) Residual() Balance {
	res := NewBalance()
	for _, p := range t.Postings {
		if w, ok := p.Weight(); ok {
			res.AddAmount(w)
		}
	}
	return res
}

// Balance completes and checks a transaction.
//
// At most one posting may elide its amount; that posting absorbs whatever the
// others leave over, which is how "the rest goes to checking" is written. The
// elided posting can only be inferred when the residual is in a single commodity
// — with two, there is no single amount that would balance it.
//
// It returns an error when the transaction cannot be made to balance. The
// tolerance is derived from the precision the author wrote: amounts given to the
// cent tolerate half a cent of rounding, which is what makes real-world
// split-to-the-penny transactions balance.
func (t *Transaction) Balance() error {
	var elided []*Posting
	for _, p := range t.Postings {
		if p.Amount == nil {
			elided = append(elided, p)
		}
	}
	if len(elided) > 1 {
		return &BalanceError{
			Pos: t.Where(), Txn: t,
			Msg: "more than one posting has no amount, so none can be inferred",
		}
	}

	res := t.Residual()

	if len(elided) == 1 {
		switch len(res) {
		case 0:
			// Everything already cancels; the elided posting is zero. Leave its
			// amount nil rather than inventing a commodity for it.
			return nil
		case 1:
			for cur, units := range res {
				elided[0].Amount = &Amount{Number: units.Neg(), Currency: cur}
				elided[0].Inferred = true
			}
			return nil
		default:
			return &BalanceError{
				Pos: t.Where(), Txn: t,
				Msg: "cannot infer the elided amount: the rest leaves " + res.String(),
			}
		}
	}

	// No elision: every commodity must cancel, within tolerance.
	tols := t.tolerances()
	for cur, units := range res {
		if units.Abs().GreaterThan(tols[cur]) {
			return &BalanceError{
				Pos: t.Where(), Txn: t,
				Msg: "does not balance: " + units.String() + " " + cur + " left over",
			}
		}
	}
	return nil
}

// tolerances is how much residual each commodity may carry before a transaction
// counts as unbalanced.
//
// The base tolerance is half of the least significant digit the author wrote:
// amounts given to the cent tolerate 0.005. The subtlety — and the reason a naive
// rule rejects perfectly good ledgers — is that a posting carrying a cost or price
// balances in a *different* commodity, where its rounding is magnified by the
// rate. Buying 2.806 units at 171.06 USD costs 479.99436 USD against a cash leg
// written as 479.99: the residual is 0.004, far more than a cent, yet the entry is
// exactly right to the precision available. Scaling the unit tolerance by the rate
// (0.0005 × 171.06 ≈ 0.086) covers it. Real investment ledgers are full of these.
//
// The widest tolerance found for a commodity wins, since any posting's rounding
// could account for the residual.
func (t *Transaction) tolerances() map[string]decimal.Decimal {
	tols := map[string]decimal.Decimal{}
	widen := func(cur string, tol decimal.Decimal) {
		if cur == "" {
			return
		}
		if prev, ok := tols[cur]; !ok || tol.GreaterThan(prev) {
			tols[cur] = tol
		}
	}

	for _, p := range t.Postings {
		if p.Amount == nil {
			continue
		}
		base := tolOf(p.Amount.Number)
		switch {
		case p.Cost != nil && p.Cost.Amount != nil:
			widen(p.Cost.Amount.Currency, base.Mul(p.Cost.Amount.Number.Abs()))
		case p.Price != nil && !p.PriceTotal:
			widen(p.Price.Currency, base.Mul(p.Price.Number.Abs()))
		case p.Cost != nil && p.Cost.Total != nil:
			// A total is stated outright, so the units' precision does not scale
			// into it; it carries only its own rounding.
			widen(p.Cost.Total.Currency, tolOf(p.Cost.Total.Number))
		case p.Price != nil && p.PriceTotal:
			widen(p.Price.Currency, tolOf(p.Price.Number))
		default:
			widen(p.Amount.Currency, base)
		}
	}
	return tols
}

// tolOf returns half the least significant digit of a written amount — 0.005 for
// an amount given to the cent. Whole numbers carry no rounding, so zero.
func tolOf(d decimal.Decimal) decimal.Decimal {
	exp := d.Exponent()
	if exp >= 0 {
		return decimal.Zero
	}
	return decimal.New(5, exp-1)
}

// BalanceError reports a transaction that does not balance.
type BalanceError struct {
	Pos Position
	Txn *Transaction
	Msg string
}

func (e *BalanceError) Error() string {
	what := e.Txn.Narration
	if what == "" {
		what = e.Txn.Payee
	}
	return e.Pos.String() + ": transaction " + strconvQuote(what) + " " + e.Msg
}
