package bean

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Position locates a directive in its source, for diagnostics.
type Position struct {
	File string
	Line int
}

func (p Position) String() string {
	if p.File == "" {
		return fmt.Sprintf("line %d", p.Line)
	}
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// Date is a calendar date — beancount has no times, and comparing dates as
// instants (rather than as Y/M/D) is a common source of off-by-one bugs across
// time zones, so it is stored as its parts and only converted on demand.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate builds a Date from a time.Time, discarding the clock.
func NewDate(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// Time renders the date as midnight UTC.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// Compare orders two dates: -1, 0 or +1.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return sign(d.Year - o.Year)
	case d.Month != o.Month:
		return sign(int(d.Month) - int(o.Month))
	case d.Day != o.Day:
		return sign(d.Day - o.Day)
	}
	return 0
}

func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }
func (d Date) After(o Date) bool  { return d.Compare(o) > 0 }
func (d Date) Equal(o Date) bool  { return d.Compare(o) == 0 }
func (d Date) IsZero() bool       { return d == Date{} }

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// Amount is a number with a commodity, e.g. "-2400.00 USD". Raw keeps the source
// spelling (grouping commas, the author's precision) so a formatter can round-trip
// a file it did not have to rewrite.
type Amount struct {
	Number   decimal.Decimal
	Currency string
	Raw      string
}

func (a Amount) String() string { return a.Number.String() + " " + a.Currency }

// IsZero reports an amount with no value; the zero Amount also has no currency.
func (a Amount) IsZero() bool { return a.Number.IsZero() }

// Cost is a posting's cost basis: the per-unit or total price paid, optionally
// with the lot's acquisition date and label — `{100.00 USD}`, `{{4000 USD}}`,
// `{100.00 USD, 2021-01-05, "lot-a"}`.
type Cost struct {
	Amount *Amount // per-unit cost ({...})
	Total  *Amount // total cost ({{...}}), converted to per-unit at booking
	Date   *Date
	Label  string
}

// Account is a colon-separated account path, e.g. "Assets:US:BofA:Checking".
type Account string

// Type returns the account's root type name ("Assets", "Expenses", …).
func (a Account) Type() string {
	if i := strings.IndexByte(string(a), ':'); i >= 0 {
		return string(a[:i])
	}
	return string(a)
}

// Leaf returns the final path component ("Checking").
func (a Account) Leaf() string {
	if i := strings.LastIndexByte(string(a), ':'); i >= 0 {
		return string(a[i+1:])
	}
	return string(a)
}

// Parent returns the account one level up, or "" at the root.
func (a Account) Parent() Account {
	if i := strings.LastIndexByte(string(a), ':'); i >= 0 {
		return a[:i]
	}
	return ""
}

// MetaItem is one metadata key/value line under a directive or posting. Values
// keep their source text as well as a decoded form, and the slice preserves source
// order — metadata is user-authored and round-tripping it must not reshuffle it.
type MetaItem struct {
	Key   string
	Value any    // string, decimal.Decimal, Date, Account, bool, *Amount, or nil
	Raw   string // source text of the value
}

// Meta is a directive's metadata, in source order.
type Meta []MetaItem

// Get returns the first value for key.
func (m Meta) Get(key string) (any, bool) {
	for _, item := range m {
		if item.Key == key {
			return item.Value, true
		}
	}
	return nil, false
}

// String returns the first value for key as a string, when it is one.
func (m Meta) String(key string) (string, bool) {
	v, ok := m.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Directive is any dated entry in a ledger.
type Directive interface {
	// When reports the directive's date. Undated entries (option, plugin,
	// include) report the zero Date and sort before everything.
	When() Date
	// Where reports the source position.
	Where() Position
}

// base carries what every directive has.
type base struct {
	Date Date
	Pos  Position
	Meta Meta
}

func (b base) When() Date      { return b.Date }
func (b base) Where() Position { return b.Pos }
func (b base) Metadata() Meta  { return b.Meta }

// Posting is one leg of a transaction. Amount is nil when the source elided it,
// in which case processing infers it; Inferred records that it did.
type Posting struct {
	Flag       string
	Account    Account
	Amount     *Amount
	Cost       *Cost
	Price      *Amount
	PriceTotal bool // the price was written @@ (total) rather than @ (per unit)
	Inferred   bool
	Meta       Meta
	Pos        Position
}

// Transaction is a dated, flagged set of postings that must balance.
type Transaction struct {
	base
	Flag      string // "*" cleared, "!" pending, or a custom flag
	Payee     string
	Narration string
	Tags      []string
	Links     []string
	Postings  []*Posting
}

// Open declares an account, optionally constraining its currencies and booking.
type Open struct {
	base
	Account    Account
	Currencies []string
	Booking    string
}

// Close ends an account's life.
type Close struct {
	base
	Account Account
}

// Commodity declares a currency/commodity, carrying metadata about it.
type Commodity struct {
	base
	Currency string
}

// Assertion checks an account's balance on a date, within an optional tolerance
// — beancount writes it as the `balance` directive. It is named for what it does
// so that Balance can mean a running total, which is the far more common noun.
type Assertion struct {
	base
	Account   Account
	Amount    Amount
	Tolerance *decimal.Decimal
}

// Pad inserts an automatic transaction to make the next Balance assertion hold.
type Pad struct {
	base
	Account Account
	Source  Account
}

// Price records an exchange rate on a date.
type Price struct {
	base
	Currency string
	Amount   Amount
}

// Note attaches dated free text to an account.
type Note struct {
	base
	Account Account
	Comment string
}

// Document links a file to an account on a date.
type Document struct {
	base
	Account Account
	Path    string
}

// Event records a dated change to some named piece of state ("location").
type Event struct {
	base
	Type        string
	Description string
}

// Custom is the generic escape hatch directive.
type Custom struct {
	base
	Type   string
	Values []string
}

// Query stores a named query in the ledger.
type Query struct {
	base
	Name string
	SQL  string
}

// Option sets a ledger-wide option. Options are undated.
type Option struct {
	base
	Name  string
	Value string
}

// Plugin requests a plugin. Parsed and preserved, never executed.
type Plugin struct {
	base
	Name   string
	Config string
}

// Include pulls in another file. Resolved by Load, which splices the included
// file's directives into the parent's.
type Include struct {
	base
	Path string
}

// File is one parsed source file: its directives in source order, plus the
// options and includes it declared.
type File struct {
	Path       string
	Directives []Directive
}

// Sorted returns the directives ordered by date, preserving source order within a
// date. Processing depends on this: a balance assertion must see every transaction
// dated before it, whatever order the file lists them in.
func (f *File) Sorted() []Directive {
	out := make([]Directive, len(f.Directives))
	copy(out, f.Directives)
	stableSortByDate(out)
	return out
}

// stableSortByDate is an insertion sort: ledgers are usually already in date
// order, which this handles in a single pass, and it is stable by construction.
func stableSortByDate(ds []Directive) {
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j].When().Before(ds[j-1].When()); j-- {
			ds[j], ds[j-1] = ds[j-1], ds[j]
		}
	}
}

// monthOf converts a 1-based month number to a time.Month.
func monthOf(m int) time.Month { return time.Month(m) }
