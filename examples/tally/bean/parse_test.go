package bean

import (
	"os"
	"testing"
)

// parseString is the test shorthand: parse and fail on any error.
func parseString(t *testing.T, src string) *File {
	t.Helper()
	f, err := Parse("test.beancount", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestParseTransaction(t *testing.T) {
	f := parseString(t, `
2021-01-04 * "RiverBank Properties" "Paying the rent"
  Assets:US:BofA:Checking  -2400.00 USD
  Expenses:Home:Rent        2400.00 USD
`)
	if len(f.Directives) != 1 {
		t.Fatalf("got %d directives, want 1", len(f.Directives))
	}
	txn, ok := f.Directives[0].(*Transaction)
	if !ok {
		t.Fatalf("got %T, want *Transaction", f.Directives[0])
	}
	if got := txn.When().String(); got != "2021-01-04" {
		t.Errorf("date = %s", got)
	}
	if txn.Flag != "*" {
		t.Errorf("flag = %q", txn.Flag)
	}
	if txn.Payee != "RiverBank Properties" || txn.Narration != "Paying the rent" {
		t.Errorf("payee/narration = %q / %q", txn.Payee, txn.Narration)
	}
	if len(txn.Postings) != 2 {
		t.Fatalf("got %d postings, want 2", len(txn.Postings))
	}
	p := txn.Postings[0]
	if p.Account != "Assets:US:BofA:Checking" {
		t.Errorf("account = %q", p.Account)
	}
	if p.Amount == nil || p.Amount.Number.String() != "-2400" || p.Amount.Currency != "USD" {
		t.Errorf("amount = %v", p.Amount)
	}
	if p.Amount.Raw != "-2400.00" {
		t.Errorf("raw = %q, want the source spelling for round-tripping", p.Amount.Raw)
	}
}

// TestParseNarrationOnly covers the single-string form, where the string is the
// narration and there is no payee.
func TestParseNarrationOnly(t *testing.T) {
	f := parseString(t, `2021-01-04 * "Monthly bank fee"
  Expenses:Financial:Fees  4.00 USD
  Assets:US:BofA:Checking
`)
	txn := f.Directives[0].(*Transaction)
	if txn.Payee != "" || txn.Narration != "Monthly bank fee" {
		t.Errorf("payee/narration = %q / %q", txn.Payee, txn.Narration)
	}
	// The elided posting parses with a nil amount, to be inferred later.
	if len(txn.Postings) != 2 {
		t.Fatalf("got %d postings", len(txn.Postings))
	}
	if txn.Postings[1].Amount != nil {
		t.Errorf("second posting should have an elided amount, got %v", txn.Postings[1].Amount)
	}
}

func TestParseTagsLinksAndFlags(t *testing.T) {
	f := parseString(t, `2021-03-01 ! "Payee" "Note" #trip #food ^receipt-1
  Assets:Cash  -10.00 USD
  ! Expenses:Food  10.00 USD
`)
	txn := f.Directives[0].(*Transaction)
	if txn.Flag != "!" {
		t.Errorf("flag = %q, want !", txn.Flag)
	}
	if len(txn.Tags) != 2 || txn.Tags[0] != "trip" || txn.Tags[1] != "food" {
		t.Errorf("tags = %v", txn.Tags)
	}
	if len(txn.Links) != 1 || txn.Links[0] != "receipt-1" {
		t.Errorf("links = %v", txn.Links)
	}
	if txn.Postings[1].Flag != "!" {
		t.Errorf("posting flag = %q", txn.Postings[1].Flag)
	}
}

func TestParseCostAndPrice(t *testing.T) {
	f := parseString(t, `2021-05-01 * "Buy shares"
  Assets:US:ETrade:GLD     10 GLD {180.00 USD}
  Assets:US:ETrade:ITOT     5 ITOT {{500.00 USD}}
  Assets:US:ETrade:VEA      3 VEA {40.00 USD, 2021-04-01, "lot-a"}
  Assets:US:ETrade:Cash  -100.00 USD @ 1.25 CAD
  Assets:Other           -200.00 USD @@ 250.00 CAD
`)
	txn := f.Directives[0].(*Transaction)
	if len(txn.Postings) != 5 {
		t.Fatalf("got %d postings, want 5", len(txn.Postings))
	}

	if c := txn.Postings[0].Cost; c == nil || c.Amount == nil || c.Amount.Number.String() != "180" {
		t.Errorf("per-unit cost = %+v", c)
	}
	if c := txn.Postings[1].Cost; c == nil || c.Total == nil || c.Total.Number.String() != "500" {
		t.Errorf("total cost = %+v", c)
	}
	c := txn.Postings[2].Cost
	if c == nil || c.Date == nil || c.Date.String() != "2021-04-01" || c.Label != "lot-a" {
		t.Errorf("cost with date/label = %+v", c)
	}
	p := txn.Postings[3]
	if p.Price == nil || p.Price.Currency != "CAD" || p.PriceTotal {
		t.Errorf("per-unit price = %+v total=%v", p.Price, p.PriceTotal)
	}
	p = txn.Postings[4]
	if p.Price == nil || !p.PriceTotal {
		t.Errorf("total price = %+v total=%v", p.Price, p.PriceTotal)
	}
}

func TestParseDirectives(t *testing.T) {
	f := parseString(t, `
option "title" "Example"
plugin "beancount.plugins.auto"
2021-01-01 open Assets:US:BofA:Checking USD
2021-01-01 open Assets:Multi USD,EUR "FIFO"
2021-12-31 close Assets:Old
2021-01-01 commodity USD
  name: "US Dollar"
2021-02-01 balance Assets:US:BofA:Checking  3793.56 USD
2021-02-01 pad Assets:X Equity:Opening-Balances
2021-03-01 price GLD  180.00 USD
2021-03-02 note Assets:X "a note"
2021-03-03 document Assets:X "/tmp/doc.pdf"
2021-03-04 event "location" "Paris"
`)
	want := []any{
		&Option{}, &Plugin{}, &Open{}, &Open{}, &Close{}, &Commodity{},
		&Assertion{}, &Pad{}, &Price{}, &Note{}, &Document{}, &Event{},
	}
	if len(f.Directives) != len(want) {
		for i, d := range f.Directives {
			t.Logf("  [%d] %T", i, d)
		}
		t.Fatalf("got %d directives, want %d", len(f.Directives), len(want))
	}
	for i, w := range want {
		if got, wantT := typeName(f.Directives[i]), typeName(w); got != wantT {
			t.Errorf("directive %d is %s, want %s", i, got, wantT)
		}
	}

	if op := f.Directives[0].(*Option); op.Name != "title" || op.Value != "Example" {
		t.Errorf("option = %+v", op)
	}
	if o := f.Directives[2].(*Open); o.Account != "Assets:US:BofA:Checking" ||
		len(o.Currencies) != 1 || o.Currencies[0] != "USD" {
		t.Errorf("open = %+v", o)
	}
	if o := f.Directives[3].(*Open); o.Booking != "FIFO" {
		t.Errorf("booking = %q", o.Booking)
	}
	if c := f.Directives[5].(*Commodity); len(c.Meta) != 1 || c.Meta[0].Key != "name" {
		t.Errorf("commodity metadata = %+v", c.Meta)
	}
	if bal := f.Directives[6].(*Assertion); bal.Amount.Number.String() != "3793.56" {
		t.Errorf("balance = %+v", bal.Amount)
	}
	if pr := f.Directives[8].(*Price); pr.Currency != "GLD" || pr.Amount.Currency != "USD" {
		t.Errorf("price = %+v", pr)
	}
}

func TestParseMetadataAndComments(t *testing.T) {
	f := parseString(t, `; a leading comment
2021-01-04 * "Payee; not a comment" "Narration"  ; trailing comment
  key: "value"
  num: 42
  when: 2021-05-05
  acct: Assets:Cash
  yes: TRUE
  Assets:Cash  -10.00 USD
    posting-key: "on the posting"
  Expenses:Food  10.00 USD
`)
	txn := f.Directives[0].(*Transaction)
	if txn.Payee != "Payee; not a comment" {
		t.Errorf("a semicolon inside a string must not start a comment: %q", txn.Payee)
	}
	if len(txn.Meta) != 5 {
		t.Fatalf("got %d metadata items, want 5: %+v", len(txn.Meta), txn.Meta)
	}
	if v, _ := txn.Meta.String("key"); v != "value" {
		t.Errorf("key = %q", v)
	}
	if v, _ := txn.Meta.Get("when"); v != (Date{2021, 5, 5}) {
		t.Errorf("when = %v", v)
	}
	if v, _ := txn.Meta.Get("acct"); v != Account("Assets:Cash") {
		t.Errorf("acct = %v", v)
	}
	if v, _ := txn.Meta.Get("yes"); v != true {
		t.Errorf("yes = %v", v)
	}
	// Metadata indented under a posting belongs to that posting, not the txn.
	if len(txn.Postings[0].Meta) != 1 || txn.Postings[0].Meta[0].Key != "posting-key" {
		t.Errorf("posting metadata = %+v", txn.Postings[0].Meta)
	}
}

func TestParsePushPopTag(t *testing.T) {
	f := parseString(t, `pushtag #trip
2021-01-01 * "One"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
poptag #trip
2021-01-02 * "Two"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
`)
	if len(f.Directives) != 2 {
		t.Fatalf("got %d directives, want 2", len(f.Directives))
	}
	one := f.Directives[0].(*Transaction)
	two := f.Directives[1].(*Transaction)
	if len(one.Tags) != 1 || one.Tags[0] != "trip" {
		t.Errorf("pushed tag not applied: %v", one.Tags)
	}
	if len(two.Tags) != 0 {
		t.Errorf("tag still applied after poptag: %v", two.Tags)
	}
}

// TestParseIsErrorTolerant checks that one bad line does not lose the rest of the
// file — a ledger is a user's data, and a syntax error must not make it unopenable.
func TestParseIsErrorTolerant(t *testing.T) {
	f, err := Parse("t.beancount", `2021-01-01 * "Good"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
2021-01-02 nonsense-directive whatever
2021-01-03 * "Also good"
  Assets:Cash  -2.00 USD
  Expenses:X    2.00 USD
`)
	if err == nil {
		t.Error("expected the bad line to be reported")
	}
	if f == nil {
		t.Fatal("parse returned no file")
	}
	txns := 0
	for _, d := range f.Directives {
		if _, ok := d.(*Transaction); ok {
			txns++
		}
	}
	if txns != 2 {
		t.Errorf("got %d transactions, want both good ones", txns)
	}
}

func TestSortedByDate(t *testing.T) {
	f := parseString(t, `2021-03-01 * "Third"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
2021-01-01 * "First"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
2021-02-01 * "Second"
  Assets:Cash  -1.00 USD
  Expenses:X    1.00 USD
`)
	sorted := f.Sorted()
	want := []string{"First", "Second", "Third"}
	for i, w := range want {
		if got := sorted[i].(*Transaction).Narration; got != w {
			t.Errorf("position %d = %q, want %q", i, got, w)
		}
	}
}

// TestParseRealLedger runs the parser over a realistic ~6k-line ledger and checks
// it comes back whole: every line accounted for, no errors, and the directive mix
// a real personal ledger has.
func TestParseRealLedger(t *testing.T) {
	src, err := os.ReadFile("../demo/example.beancount")
	if err != nil {
		t.Skipf("no sample ledger: %v", err)
	}
	f, err := Parse("example.beancount", string(src))
	if err != nil {
		t.Fatalf("parse reported errors: %v", err)
	}

	counts := map[string]int{}
	for _, d := range f.Directives {
		counts[typeName(d)]++
	}
	t.Logf("directives: %v", counts)

	// These counts were cross-checked against the reference beancount engine as a
	// black-box oracle: run both over this file, compare the tallies. They agreed
	// exactly, so the numbers are pinned here — a regression that drops or invents
	// directives shows up immediately, and the check needs no dependency on that
	// engine (which is GPL and must never be linked into a shipped build).
	want := map[string]int{
		"*bean.Transaction": 1007,
		"*bean.Open":        63,
		"*bean.Price":       810,
		"*bean.Assertion":   78,
		"*bean.Commodity":   10,
		"*bean.Event":       5,
		"*bean.Option":      2,
	}
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("%s: parsed %d, want %d", kind, counts[kind], n)
		}
	}

	// Postings are the finer-grained check: the same directive count with the
	// wrong posting count would mean lines are being silently dropped.
	postings := 0
	for _, d := range f.Directives {
		if txn, ok := d.(*Transaction); ok {
			postings += len(txn.Postings)
		}
	}
	if postings != 3094 {
		t.Errorf("parsed %d postings, want 3094 (oracle-verified)", postings)
	}

	// Every transaction must have at least two postings, and at most one elided
	// amount (more than one cannot be inferred).
	for _, d := range f.Directives {
		txn, ok := d.(*Transaction)
		if !ok {
			continue
		}
		if len(txn.Postings) < 2 {
			t.Errorf("%s: transaction %q has %d postings",
				txn.Where(), txn.Narration, len(txn.Postings))
		}
		elided := 0
		for _, p := range txn.Postings {
			if p.Amount == nil {
				elided++
			}
		}
		if elided > 1 {
			t.Errorf("%s: transaction %q has %d elided amounts", txn.Where(), txn.Narration, elided)
		}
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *Transaction:
		return "*bean.Transaction"
	case *Open:
		return "*bean.Open"
	case *Close:
		return "*bean.Close"
	case *Commodity:
		return "*bean.Commodity"
	case *Assertion:
		return "*bean.Assertion"
	case *Pad:
		return "*bean.Pad"
	case *Price:
		return "*bean.Price"
	case *Note:
		return "*bean.Note"
	case *Document:
		return "*bean.Document"
	case *Event:
		return "*bean.Event"
	case *Custom:
		return "*bean.Custom"
	case *Query:
		return "*bean.Query"
	case *Option:
		return "*bean.Option"
	case *Plugin:
		return "*bean.Plugin"
	case *Include:
		return "*bean.Include"
	}
	return "unknown"
}
