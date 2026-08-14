package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doug/tally/decimal"
)

const editable = `; A hand-kept ledger.
option "title" "Test"

* Accounts
2021-01-01 open Assets:Cash USD
2021-01-01 open Expenses:Food USD

* Transactions
2021-01-10 * "Grocer" "Weekly shop"
  Assets:Cash      -50.00 USD
  Expenses:Food     50.00 USD
`

func writeLedger(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.beancount")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func entry(day int, amount string) NewEntry {
	return NewEntry{
		Date:      time.Date(2021, time.January, day, 0, 0, 0, 0, time.UTC),
		Payee:     "Cafe",
		Narration: "Coffee",
		From:      "Assets:Cash",
		To:        "Expenses:Food",
		Amount:    decimal.RequireFromString(amount),
		Currency:  "USD",
	}
}

// TestAddWritesToDiskAndReloads is the whole point of the feature: a transaction
// entered in the app ends up in the user's file, and reopening that file shows it.
func TestAddWritesToDiskAndReloads(t *testing.T) {
	path := writeLedger(t, editable)
	b, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !b.Writable() {
		t.Fatal("a ledger opened from a real file should be writable")
	}

	res, err := b.Add(entry(20, "4.50"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !res.Saved {
		t.Error("Add did not report saving")
	}
	if len(res.Invalidated) != 0 {
		t.Errorf("nothing should have been invalidated: %v", res.Invalidated)
	}

	// The in-memory ledger reflects it immediately.
	if got := b.led.BalanceOf("Expenses:Food").Get("USD"); !got.Equal(decimal.RequireFromString("54.50")) {
		t.Errorf("in-memory food balance = %s, want 54.50", got)
	}

	// And so does the file, reopened from scratch.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.led.BalanceOf("Expenses:Food").Get("USD"); !got.Equal(decimal.RequireFromString("54.50")) {
		t.Errorf("reloaded food balance = %s, want 54.50", got)
	}

	// The user's comments and headings survived the write.
	raw, _ := os.ReadFile(path)
	for _, marker := range []string{"; A hand-kept ledger.", "* Accounts", "* Transactions", "Weekly shop"} {
		if !strings.Contains(string(raw), marker) {
			t.Errorf("saving destroyed %q", marker)
		}
	}
	if !strings.Contains(string(raw), `"Cafe" "Coffee"`) {
		t.Error("the new entry is not in the file")
	}
}

// TestAddReportsInvalidatedAssertions checks the consequence-reporting: inserting
// before a balance assertion makes it fail, and the user must be told.
func TestAddReportsInvalidatedAssertions(t *testing.T) {
	path := writeLedger(t, editable+`
2021-02-01 balance Assets:Cash  -50.00 USD
`)
	b, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(b.Problems()) != 0 {
		t.Fatalf("the ledger should start clean: %v", b.Problems())
	}

	res, err := b.Add(entry(20, "4.50"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(res.Invalidated) != 1 {
		t.Fatalf("expected one invalidated assertion, got %v", res.Invalidated)
	}
	if !strings.Contains(res.Invalidated[0], "Assets:Cash") {
		t.Errorf("the report should name the account: %q", res.Invalidated[0])
	}
}

// TestAddDoesNotReportPreexistingProblems: a user who already has a failing
// assertion should not be blamed for it by an unrelated edit.
func TestAddDoesNotReportPreexistingProblems(t *testing.T) {
	path := writeLedger(t, editable+`
2021-02-01 balance Assets:Cash  -999.00 USD
`)
	b, _ := Open(path)
	if len(b.Problems()) != 1 {
		t.Fatalf("expected one pre-existing failure, got %v", b.Problems())
	}

	res, err := b.Add(entry(20, "4.50"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// The assertion's message changes (the numbers move), so it counts as new —
	// but only one, and it must still concern the account the user touched.
	for _, msg := range res.Invalidated {
		if !strings.Contains(msg, "Assets:Cash") {
			t.Errorf("unrelated problem reported as caused by the edit: %q", msg)
		}
	}
}

func TestAddValidates(t *testing.T) {
	path := writeLedger(t, editable)
	b, _ := Open(path)

	cases := []struct {
		name string
		mut  func(*NewEntry)
		want string
	}{
		{"no date", func(e *NewEntry) { e.Date = time.Time{} }, "date"},
		{"no from", func(e *NewEntry) { e.From = "" }, "comes from"},
		{"no to", func(e *NewEntry) { e.To = "" }, "goes to"},
		{"same accounts", func(e *NewEntry) { e.To = e.From }, "must differ"},
		{"zero amount", func(e *NewEntry) { e.Amount = decimal.Zero }, "amount"},
		{"negative", func(e *NewEntry) { e.Amount = decimal.RequireFromString("-5") }, "positive"},
		{"no currency", func(e *NewEntry) { e.Currency = "" }, "currency"},
	}
	for _, tc := range cases {
		e := entry(20, "4.50")
		tc.mut(&e)
		_, err := b.Add(e)
		if err == nil {
			t.Errorf("%s: expected a validation error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q should mention %q", tc.name, err, tc.want)
		}
	}

	// A rejected entry must not have touched the file.
	raw, _ := os.ReadFile(path)
	if string(raw) != editable {
		t.Error("a failed validation modified the ledger")
	}
}

// TestEmbeddedLedgerIsEditableButNotWritable: the bundled demo can be experimented
// with, but must never try to write to a path that isn't a file.
func TestEmbeddedLedgerIsEditableButNotWritable(t *testing.T) {
	b, err := OpenBytes("demo.beancount", []byte(editable))
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if !b.CanEdit() {
		t.Error("an in-memory ledger should still be editable")
	}
	if b.Writable() {
		t.Error("an in-memory ledger must not report itself writable")
	}

	res, err := b.Add(entry(20, "4.50"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Saved {
		t.Error("an in-memory ledger must not report saving")
	}
	if got := b.led.BalanceOf("Expenses:Food").Get("USD"); !got.Equal(decimal.RequireFromString("54.50")) {
		t.Errorf("in-memory edit did not apply: %s", got)
	}
}

// TestAddOrdersByDate checks entries land chronologically however they arrive.
func TestAddOrdersByDate(t *testing.T) {
	path := writeLedger(t, editable)
	b, _ := Open(path)

	for _, day := range []int{25, 15, 20} {
		e := entry(day, "1.00")
		e.Narration = "day-" + itoa(day)
		if _, err := b.Add(e); err != nil {
			t.Fatalf("Add day %d: %v", day, err)
		}
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	i15, i20, i25 := strings.Index(text, "day-15"), strings.Index(text, "day-20"), strings.Index(text, "day-25")
	if i15 < 0 || i20 < 0 || i25 < 0 {
		t.Fatal("not all entries were written")
	}
	if !(i15 < i20 && i20 < i25) {
		t.Errorf("entries are out of date order: 15@%d 20@%d 25@%d", i15, i20, i25)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
