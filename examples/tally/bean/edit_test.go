package bean

import (
	"os"
	"strings"
	"testing"
)

func TestFormatTransactionAlignsAmounts(t *testing.T) {
	f := parseString(t, `2021-01-04 * "RiverBank Properties" "Paying the rent"
  Assets:US:BofA:Checking  -2400.00 USD
  Expenses:Home:Rent        2400.00 USD
`)
	got := FormatTransaction(f.Directives[0].(*Transaction), FormatOptions{})
	want := `2021-01-04 * "RiverBank Properties" "Paying the rent"
  Assets:US:BofA:Checking  -2400.00 USD
  Expenses:Home:Rent        2400.00 USD
`
	if got != want {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatRoundTrips is the property that makes the formatter safe to use: an
// entry that goes through parse → format → parse comes back the same.
func TestFormatRoundTrips(t *testing.T) {
	sources := []string{
		`2021-01-04 * "Payee" "Narration"
  Assets:Cash  -10.00 USD
  Expenses:Food  10.00 USD
`,
		`2021-03-01 ! "Pending" "Note" #trip #food ^receipt-1
  key: "value"
  Assets:Cash  -10.00 USD
    posting-key: "on the posting"
  ! Expenses:Food  10.00 USD
`,
		`2021-05-01 * "Buy shares"
  Assets:ETrade:GLD  10 GLD {180.00 USD, 2021-04-01, "lot-a"}
  Assets:ETrade:VEA  5 VEA {{500.00 USD}}
  Assets:Cash  -100.00 USD @ 1.25 CAD
  Assets:Other  -200.00 USD @@ 250.00 CAD
  Assets:Rest
`,
		`2021-06-01 * "Narration only"
  Assets:Cash  -1.00 USD
  Expenses:X  1.00 USD
`,
	}
	for i, src := range sources {
		first := parseString(t, src).Directives[0].(*Transaction)
		text := FormatTransaction(first, FormatOptions{})
		again, err := Parse("round.beancount", text)
		if err != nil {
			t.Fatalf("case %d: reparsing our own output failed: %v\n%s", i, err, text)
		}
		if len(again.Directives) != 1 {
			t.Fatalf("case %d: reparse produced %d directives:\n%s", i, len(again.Directives), text)
		}
		second := again.Directives[0].(*Transaction)
		if diff := txnDiff(first, second); diff != "" {
			t.Errorf("case %d: round trip changed the entry: %s\n%s", i, diff, text)
		}
	}
}

// txnDiff compares two transactions structurally, returning "" when equal.
func txnDiff(a, b *Transaction) string {
	if !a.Date.Equal(b.Date) {
		return "date " + a.Date.String() + " vs " + b.Date.String()
	}
	if a.Flag != b.Flag {
		return "flag " + a.Flag + " vs " + b.Flag
	}
	if a.Payee != b.Payee || a.Narration != b.Narration {
		return "payee/narration " + a.Payee + "/" + a.Narration + " vs " + b.Payee + "/" + b.Narration
	}
	if strings.Join(a.Tags, ",") != strings.Join(b.Tags, ",") {
		return "tags"
	}
	if strings.Join(a.Links, ",") != strings.Join(b.Links, ",") {
		return "links"
	}
	if len(a.Postings) != len(b.Postings) {
		return "posting count"
	}
	for i := range a.Postings {
		pa, pb := a.Postings[i], b.Postings[i]
		if pa.Account != pb.Account || pa.Flag != pb.Flag {
			return "posting " + string(pa.Account)
		}
		if (pa.Amount == nil) != (pb.Amount == nil) {
			return "posting amount presence on " + string(pa.Account)
		}
		if pa.Amount != nil {
			if !pa.Amount.Number.Equal(pb.Amount.Number) || pa.Amount.Currency != pb.Amount.Currency {
				return "posting amount on " + string(pa.Account)
			}
		}
		if (pa.Cost == nil) != (pb.Cost == nil) {
			return "cost presence on " + string(pa.Account)
		}
		if (pa.Price == nil) != (pb.Price == nil) || pa.PriceTotal != pb.PriceTotal {
			return "price on " + string(pa.Account)
		}
		if len(pa.Meta) != len(pb.Meta) {
			return "posting metadata on " + string(pa.Account)
		}
	}
	if len(a.Meta) != len(b.Meta) {
		return "metadata count"
	}
	return ""
}

// ledgerWithComments is a file kept the way people actually keep them: section
// headings, explanatory comments, and blank lines that group entries.
const ledgerWithComments = `; My ledger — hand-maintained since 2019.
option "title" "Personal"

* Accounts

2021-01-01 open Assets:Cash USD
2021-01-01 open Expenses:Food USD

* Transactions

; The rent went up this year, hence the odd amount.
2021-01-04 * "Landlord" "Rent"
  Assets:Cash     -2400.00 USD
  Expenses:Rent    2400.00 USD

2021-01-10 * "Grocer" "Weekly shop"
  Assets:Cash      -50.00 USD
  Expenses:Food     50.00 USD
`

// TestInsertPreservesEverythingElse is the promise the editor has to keep: adding
// an entry must not disturb one byte of the user's hand-maintained file.
func TestInsertPreservesEverythingElse(t *testing.T) {
	src, err := NewSource("l.beancount", ledgerWithComments)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	before := src.String()

	txn := &Transaction{
		base:      base{Date: Date{2021, 1, 20}},
		Flag:      "*",
		Payee:     "Cafe",
		Narration: "Coffee",
		Postings: []*Posting{
			{Account: "Assets:Cash", Amount: &Amount{Number: dec("-4.50"), Currency: "USD", Raw: "-4.50"}},
			{Account: "Expenses:Food", Amount: &Amount{Number: dec("4.50"), Currency: "USD", Raw: "4.50"}},
		},
	}
	src.Insert(txn, FormatOptions{})
	after := src.String()

	// Every original line must still be present, in order.
	for _, line := range splitLines(before) {
		if !strings.Contains(after, line) {
			t.Fatalf("insert lost the line %q", line)
		}
	}
	for _, marker := range []string{
		"; My ledger — hand-maintained since 2019.",
		"* Accounts",
		"* Transactions",
		"; The rent went up this year, hence the odd amount.",
	} {
		if !strings.Contains(after, marker) {
			t.Errorf("insert destroyed %q", marker)
		}
	}
	if !strings.Contains(after, `"Cafe" "Coffee"`) {
		t.Error("the new entry is missing")
	}

	// It must land in date order, after the 10th.
	iTen := strings.Index(after, "Weekly shop")
	iNew := strings.Index(after, "Coffee")
	if iNew < iTen {
		t.Error("the new entry was not inserted in date order")
	}

	// And the result must still parse into one more transaction than before.
	led, err := src.Ledger()
	if err != nil {
		t.Fatalf("edited source does not parse: %v", err)
	}
	if len(led.Problems) != 0 {
		t.Errorf("edited source has problems: %v", led.Problems)
	}
	txns := 0
	for _, d := range led.Directives {
		if _, ok := d.(*Transaction); ok {
			txns++
		}
	}
	if txns != 3 {
		t.Errorf("got %d transactions after insert, want 3", txns)
	}
}

func TestInsertAppendsWhenLatest(t *testing.T) {
	src, _ := NewSource("l.beancount", ledgerWithComments)
	txn := &Transaction{
		base: base{Date: Date{2030, 1, 1}}, Flag: "*", Narration: "Future",
		Postings: []*Posting{
			{Account: "Assets:Cash", Amount: &Amount{Number: dec("-1"), Currency: "USD", Raw: "-1.00"}},
			{Account: "Expenses:Food", Amount: &Amount{Number: dec("1"), Currency: "USD", Raw: "1.00"}},
		},
	}
	src.Insert(txn, FormatOptions{})
	after := src.String()
	if strings.Index(after, "Future") < strings.Index(after, "Weekly shop") {
		t.Error("a later-dated entry should be appended after the earlier ones")
	}
}

func TestReplaceOnlyTouchesItsOwnEntry(t *testing.T) {
	src, _ := NewSource("l.beancount", ledgerWithComments)

	var target *Transaction
	for _, d := range src.Directives() {
		if txn, ok := d.(*Transaction); ok && txn.Narration == "Rent" {
			target = txn
		}
	}
	if target == nil {
		t.Fatal("could not find the rent transaction")
	}

	edited := &Transaction{
		base: base{Date: target.Date}, Flag: "*", Payee: "Landlord", Narration: "Rent (increased)",
		Postings: []*Posting{
			{Account: "Assets:Cash", Amount: &Amount{Number: dec("-2500"), Currency: "USD", Raw: "-2500.00"}},
			{Account: "Expenses:Rent", Amount: &Amount{Number: dec("2500"), Currency: "USD", Raw: "2500.00"}},
		},
	}
	if !src.Replace(target, edited, FormatOptions{}) {
		t.Fatal("Replace reported failure")
	}
	after := src.String()

	if !strings.Contains(after, "Rent (increased)") || !strings.Contains(after, "-2500.00 USD") {
		t.Error("the replacement did not take")
	}
	if strings.Contains(after, "-2400.00 USD") {
		t.Error("the old amount survived the replacement")
	}
	// The comment above it, the heading, and the untouched entry all remain.
	for _, marker := range []string{
		"; The rent went up this year, hence the odd amount.",
		"* Transactions",
		"Weekly shop",
		"  Assets:Cash      -50.00 USD",
	} {
		if !strings.Contains(after, marker) {
			t.Errorf("replace disturbed %q", marker)
		}
	}
}

func TestDeleteRemovesOnlyItsEntry(t *testing.T) {
	src, _ := NewSource("l.beancount", ledgerWithComments)
	var target Directive
	for _, d := range src.Directives() {
		if txn, ok := d.(*Transaction); ok && txn.Narration == "Weekly shop" {
			target = d
		}
	}
	if !src.Delete(target) {
		t.Fatal("Delete reported failure")
	}
	after := src.String()
	if strings.Contains(after, "Weekly shop") || strings.Contains(after, "-50.00 USD") {
		t.Error("the entry was not removed")
	}
	for _, marker := range []string{"* Transactions", "Rent", "; My ledger"} {
		if !strings.Contains(after, marker) {
			t.Errorf("delete disturbed %q", marker)
		}
	}
	// No trailing gap left behind.
	if strings.Contains(after, "\n\n\n") {
		t.Error("delete left a widening blank gap")
	}
}

// TestEditRealLedgerInvalidatesDownstreamAssertions documents what inserting into
// a real ledger actually does — and it is not "nothing".
//
// A balance assertion is the ledger's own checkpoint: "on this date this account
// held exactly this much". Inserting a $4.50 expense into a checking account makes
// every later assertion on that account wrong by $4.50, because the account really
// is $4.50 lighter from then on. That is beancount working correctly, not a bug in
// the edit — and it is something an app must *tell the user*, because their
// checkpoints now need updating.
//
// The test therefore asserts the precise blast radius: the only new problems are
// assertion failures on the account that was touched, each off by exactly the
// inserted amount, and nothing else about the ledger broke.
func TestEditRealLedgerInvalidatesDownstreamAssertions(t *testing.T) {
	raw, err := os.ReadFile("../testdata/example.beancount")
	if err != nil {
		t.Skipf("no sample ledger: %v", err)
	}
	src, err := NewSource("example.beancount", string(raw))
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	const account = "Assets:US:BofA:Checking"
	txn := &Transaction{
		base: base{Date: Date{2022, 6, 15}}, Flag: "*", Payee: "Cafe Mocha", Narration: "Coffee",
		Postings: []*Posting{
			{Account: account, Amount: &Amount{Number: dec("-4.50"), Currency: "USD", Raw: "-4.50"}},
			{Account: "Expenses:Food:Coffee", Amount: &Amount{Number: dec("4.50"), Currency: "USD", Raw: "4.50"}},
		},
	}
	// The expense account already carries history, so the invariant to check is
	// the delta, not the total.
	beforeLed, _ := src.Ledger()
	beforeExpense := beforeLed.BalanceOf("Expenses:Food:Coffee").Get("USD")

	src.Insert(txn, FormatOptions{})

	led, err := src.Ledger()
	if err != nil {
		t.Fatalf("the edited ledger no longer parses: %v", err)
	}

	// Every problem must be an assertion on the touched account, off by the
	// inserted amount. Anything else means the edit damaged something.
	for _, p := range led.Problems {
		ae, ok := p.(*AssertionError)
		if !ok {
			t.Errorf("unexpected problem after edit: %v", p)
			continue
		}
		if ae.Account != account {
			t.Errorf("edit disturbed an untouched account: %v", ae)
		}
		if !ae.Diff.Abs().Equal(dec("4.50")) {
			t.Errorf("assertion off by %s, want 4.50: %v", ae.Diff, ae)
		}
	}
	if len(led.Problems) == 0 {
		t.Error("expected the later balance assertions on this account to be invalidated")
	}

	// The balance moved by exactly the inserted amount.
	if got := led.BalanceOf("Expenses:Food:Coffee").Get("USD").Sub(beforeExpense); !got.Equal(dec("4.50")) {
		t.Errorf("expense moved by %s, want 4.50", got)
	}

	// The file grew by only the new entry (blank separator + header + 2 postings).
	before, after := len(splitLines(string(raw))), len(splitLines(src.String()))
	if grew := after - before; grew != 4 {
		t.Errorf("file grew by %d lines, want 4", grew)
	}
}

// TestEditCleanWhereNoAssertionsFollow is the complementary case: inserting into
// an account the ledger does not checkpoint leaves it entirely clean, which is
// what makes the failure above attributable to the assertions rather than the edit.
func TestEditCleanWhereNoAssertionsFollow(t *testing.T) {
	src, err := NewSource("l.beancount", ledgerWithComments)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	txn := &Transaction{
		base: base{Date: Date{2021, 1, 20}}, Flag: "*", Payee: "Cafe", Narration: "Coffee",
		Postings: []*Posting{
			{Account: "Assets:Cash", Amount: &Amount{Number: dec("-4.50"), Currency: "USD", Raw: "-4.50"}},
			{Account: "Expenses:Food", Amount: &Amount{Number: dec("4.50"), Currency: "USD", Raw: "4.50"}},
		},
	}
	src.Insert(txn, FormatOptions{})

	led, err := src.Ledger()
	if err != nil {
		t.Fatalf("edited source does not parse: %v", err)
	}
	if len(led.Problems) != 0 {
		t.Errorf("a ledger with no downstream assertions should stay clean: %v", led.Problems)
	}
	if got := led.BalanceOf("Expenses:Food").Get("USD"); !got.Equal(dec("54.50")) {
		t.Errorf("food after insert = %s, want 54.50", got)
	}
}
