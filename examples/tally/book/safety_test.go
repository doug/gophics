package book

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doug/tally/decimal"
	"github.com/doug/tally/demo"
)

// The embedded demo's Path is the bare name "example.beancount". Whether a
// ledger can be written used to be decided by stat'ing Path, so a file of that
// name in the working directory — the demo directory itself, or a saved copy
// of the sample — was silently overwritten with the whole demo ledger.
func TestEmbeddedLedgerNeverWritesToTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, demo.Name)
	const original = "; not a ledger\n"
	if err := os.WriteFile(victim, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	b, err := OpenBytes(demo.Name, demo.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if b.Writable() {
		t.Error("an embedded ledger reports itself writable because a file of its name exists")
	}
	res, err := b.Add(NewEntry{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		From: "Assets:US:BofA:Checking", To: "Expenses:Food:Groceries",
		Amount: decimal.RequireFromString("1.00"), Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved {
		t.Error("Add reported saving an embedded ledger")
	}
	if after, _ := os.ReadFile(victim); string(after) != original {
		t.Fatalf("the unrelated file was overwritten: now %d bytes", len(after))
	}
}

// A failed write must leave the in-memory ledger as it was: an entry that shows
// as added but never reached disk is a lie, and Save again would insert it
// twice.
func TestAddRollsBackWhenTheWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory, so the failure cannot be provoked")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.beancount")
	if err := os.WriteFile(path, []byte(editable), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	before := foodPostings(t, b)
	if err := os.Chmod(dir, 0o500); err != nil { // no new files: the temp file cannot be created
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	res, err := b.Add(entry(20, "4.50"))
	if err == nil {
		t.Fatal("Add succeeded against an unwritable directory")
	}
	if res.Saved {
		t.Error("Saved reported on a failed write")
	}
	if got := foodPostings(t, b); got != before {
		t.Errorf("the ledger holds %d Food postings after a failed save, want %d", got, before)
	}
	if strings.Contains(b.src.String(), "Coffee") {
		t.Error("the failed entry is still in the text; saving again would add it twice")
	}
	_ = os.Chmod(dir, 0o700)
	if raw, _ := os.ReadFile(path); string(raw) != editable {
		t.Error("the file changed despite the failure")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*tmp")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

func foodPostings(t *testing.T, b *Book) int {
	t.Helper()
	entries, err := b.Register("Expenses:Food", "USD")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// A ledger edited elsewhere since it was opened must not be overwritten with
// this Book's stale copy of it.
func TestAddRefusesWhenTheFileChangedOnDisk(t *testing.T) {
	path := writeLedger(t, editable)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	external := editable + "\n2021-01-12 * \"Elsewhere\"\n  Assets:Cash      -1.00 USD\n  Expenses:Food     1.00 USD\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	// Filesystems with coarse timestamps could still report the old time.
	later := b.modTime.Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add(entry(20, "4.50")); err == nil {
		t.Fatal("Add overwrote a file that changed on disk")
	}
	if raw, _ := os.ReadFile(path); string(raw) != external {
		t.Error("the external edit was lost")
	}
	// Reopening picks up the new state and edits work again.
	b2, _ := Open(path)
	if _, err := b2.Add(entry(20, "4.50")); err != nil {
		t.Fatalf("Add after reopen: %v", err)
	}
}

// The modification time alone missed an edit made within the same clock tick
// on a filesystem with coarse timestamps; the size is the second witness.
func TestAddRefusesWhenOnlyTheSizeChanged(t *testing.T) {
	path := writeLedger(t, editable)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	external := editable + "\n2021-01-12 * \"Elsewhere\"\n  Assets:Cash      -1.00 USD\n  Expenses:Food     1.00 USD\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	// Put the recorded time back, as a coarse clock would have.
	if err := os.Chtimes(path, b.modTime, b.modTime); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add(entry(20, "4.50")); !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("Add returned %v, want ErrChangedOnDisk", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != external {
		t.Error("the external edit was lost")
	}
}

// A ledger reached through a symlink is written through it. A rename over the
// link replaced the link with a regular file and left the real ledger as it
// was — while reporting the entry saved.
func TestAddWritesThroughASymlink(t *testing.T) {
	real := writeLedger(t, editable)
	link := filepath.Join(t.TempDir(), "ledger.beancount")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create a symlink here (a capability, not a defect): %v", err)
	}
	b, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if b.Path != link {
		t.Errorf("Path = %s, want the path the user opened, %s", b.Path, link)
	}
	res, err := b.Add(entry(20, "4.50"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Saved {
		t.Error("Add did not report saving")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a regular file (%v, %v)", info, err)
	}
	if raw, _ := os.ReadFile(real); !strings.Contains(string(raw), "Coffee") {
		t.Error("the entry did not reach the file the link points at")
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(link), ".*tmp")); len(left) != 0 {
		t.Errorf("temporary files left beside the link: %v", left)
	}
	// A second Add sees its own write as the current state of the target.
	if _, err := b.Add(entry(21, "1.00")); err != nil {
		t.Fatalf("second Add: %v", err)
	}
}

// A read-only ledger is not written. The rename succeeds over one on POSIX,
// so a file the user had protected was rewritten and the button still said
// "Save to ledger".
func TestReadOnlyLedgerIsNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only file, so the refusal cannot be provoked")
	}
	path := writeLedger(t, editable)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Writable() {
		t.Error("a read-only ledger reports itself writable")
	}
	res, err := b.Add(entry(20, "4.50"))
	if err == nil {
		t.Fatal("Add wrote a read-only ledger")
	}
	if res.Saved {
		t.Error("Saved reported on a refused write")
	}
	if raw, _ := os.ReadFile(path); string(raw) != editable {
		t.Error("the read-only file changed")
	}
	if strings.Contains(b.src.String(), "Coffee") {
		t.Error("the refused entry is still in the text")
	}

	// Protected after it was opened: the save notices, and the label follows.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Writable() {
		t.Fatal("a writable ledger reports itself read-only")
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add(entry(20, "4.50")); err == nil {
		t.Fatal("Add wrote a ledger made read-only after it was opened")
	}
	if b.Writable() {
		t.Error("Writable() still true after the file refused a write")
	}
}

// The write goes through a rename, so the file keeps its permissions and no
// temporary file remains.
func TestAddWritesAtomically(t *testing.T) {
	path := writeLedger(t, editable)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := Open(path)
	if _, err := b.Add(entry(20, "4.50")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions changed to %v", info.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*tmp")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
	// A second add in the same session works: the recorded modification time
	// follows the write.
	if _, err := b.Add(entry(21, "1.00")); err != nil {
		t.Fatalf("second Add: %v", err)
	}
}

// Validate refuses an account the ledger does not declare. Only the dropdown
// stood between a typo and a brand-new account before.
func TestValidateRejectsUnknownAccount(t *testing.T) {
	path := writeLedger(t, editable)
	b, _ := Open(path)
	e := entry(20, "4.50")
	e.To = "Expenses:Fodo"
	_, err := b.Add(e)
	if err == nil {
		t.Fatal("an entry to an undeclared account was accepted")
	}
	if !strings.Contains(err.Error(), "Expenses:Fodo") {
		t.Errorf("error %q does not name the account", err)
	}
	if names := b.AccountNames(); len(names) != 2 {
		t.Errorf("accounts after the rejected add: %v", names)
	}
}

// A padded account's register starts with the pad, with the running balance
// accumulated in date order.
func TestRegisterOrdersThePadByDate(t *testing.T) {
	b, err := OpenBytes("t.beancount", []byte(`2021-01-01 open Assets:Cash USD
2021-01-01 open Equity:Opening USD
2021-01-01 open Expenses:Food USD
2021-01-01 pad Assets:Cash Equity:Opening
2021-01-05 * "Lunch"
  Expenses:Food  10.00 USD
  Assets:Cash
2021-01-10 balance Assets:Cash 90.00 USD
`))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := b.Register("Assets:Cash", "USD")
	if len(entries) != 2 {
		t.Fatalf("%d register rows, want 2", len(entries))
	}
	if entries[0].Date != "2021-01-01" || !entries[0].Balance.Equal(decimal.RequireFromString("100")) {
		t.Errorf("first row = %s balance %s, want the 2021-01-01 pad at 100", entries[0].Date, entries[0].Balance)
	}
	if entries[1].Date != "2021-01-05" || !entries[1].Balance.Equal(decimal.RequireFromString("90")) {
		t.Errorf("second row = %s balance %s, want 2021-01-05 at 90", entries[1].Date, entries[1].Balance)
	}
}

// A ledger whose last transaction falls on a month end produced that point
// twice: the month end, then "the last date" again.
func TestNetWorthEndsOnceWhenLastDateIsAMonthEnd(t *testing.T) {
	b, err := OpenBytes("t", []byte(`2024-01-01 open Assets:Cash USD
2024-01-01 open Equity:Opening USD
2024-01-15 * "Seed"
  Assets:Cash  100.00 USD
  Equity:Opening
2024-01-31 * "More"
  Assets:Cash  50.00 USD
  Equity:Opening
`))
	if err != nil {
		t.Fatal(err)
	}
	pts := b.NetWorth("USD")
	if len(pts) != 1 {
		t.Fatalf("got %d points, want 1: %v", len(pts), pts)
	}
	if got := pts[0].Date.Format("2006-01-02"); got != "2024-01-31" {
		t.Errorf("point is %s, want 2024-01-31", got)
	}
}
