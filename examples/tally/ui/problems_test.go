package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
)

// The engine keeps going past a line it cannot parse, an include it cannot
// read or an assertion that fails, so the ledger still opens — which is only
// defensible if the reader is told. Nothing in the UI read Problems() before.
func TestProblemsAreSurfaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.beancount")
	src := `* Accounts | 2021
2021-01-01 open Assets:Cash USD
2021-01-01 open Expenses:Food USD
2021-01-10 * "Grocer"
  Assets:Cash      -50.00 USD
  Expenses:Food     50.00 USD
2021-02-01 balance Assets:Cash  0.00 USD
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var st *state
	stateHook = func(s *state) { st = s }
	defer func() { stateHook = nil }()
	a := apptest.New(t, App{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 1040, H: 680}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}))
	a.Render()
	if a.HasText("problem") {
		t.Fatal("the clean demo ledger shows a problems banner")
	}

	// The remembered ledger arrives with Preferences, as on a real shell —
	// which rebuilds the tree when it wires them.
	st.loadWith(apptest.NewPrefs(map[string]string{prefKeyLedger: path}))
	a.Owner().RebuildAll()
	a.Render()
	if st.book == nil || st.book.Path != path {
		t.Fatalf("the broken ledger did not load: %v", st.err)
	}
	if n := len(st.book.Problems()); n != 2 {
		t.Fatalf("%d problems, want the stray '|' and the failed assertion: %v", n, st.book.Problems())
	}
	a.AssertText("2 problems in this ledger")

	// The full list is one tap away and names both.
	a.TapText("Show")
	a.Render()
	a.AssertText("unexpected character")
	a.AssertText("balance assertion")
	a.TapText("Hide")
	a.Render()
	if a.HasText("unexpected character") {
		t.Error("the list did not close")
	}

	// The header tabs leave the list too. screen tests showProblems before
	// the view, so with the list open the tabs used to do nothing visible and
	// the banner's Hide was the only way out.
	a.TapText("Show")
	a.Render()
	a.AssertText("unexpected character")
	a.TapText("Balances")
	a.Render()
	if a.HasText("unexpected character") {
		t.Error("the Balances tab left the problems list showing")
	}
	if st.showProblems {
		t.Error("showProblems still set after a tab")
	}
	a.TapText("Show")
	a.Render()
	st.open("Assets:Cash")
	a.Render()
	if st.showProblems {
		t.Error("showProblems still set after opening a register")
	}
}

// A save refused because the file changed on disk tells the user to reload it,
// so there has to be a way to: the file picker was the only loader, and it
// means finding the same file again and losing the typed entry.
func TestReloadAfterTheFileChangedOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.beancount")
	src := `2021-01-01 open Assets:Cash USD
2021-01-01 open Expenses:Food USD

2021-01-10 * "Grocer" "Weekly shop"
  Assets:Cash      -50.00 USD
  Expenses:Food     50.00 USD
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var st *state
	stateHook = func(s *state) { st = s }
	defer func() { stateHook = nil }()
	a := apptest.New(t, App{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 1040, H: 680}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}))
	a.Render()
	st.loadWith(apptest.NewPrefs(map[string]string{prefKeyLedger: path}))
	a.Owner().RebuildAll()
	a.Render()
	if st.book == nil || st.book.Path != path {
		t.Fatalf("the ledger did not load: %v", st.err)
	}

	st.toggleForm()
	a.Render()
	st.form.date, st.form.payee, st.form.narration = "2021-01-20", "Cafe", "Coffee"
	st.form.from, st.form.to, st.form.amount = "Assets:Cash", "Expenses:Food", "4.50"

	// Edited elsewhere since the open, with the timestamp moved past a coarse
	// clock's resolution so the check does not depend on the filesystem.
	external := src + "\n2021-01-12 * \"Elsewhere\"\n  Assets:Cash      -1.00 USD\n  Expenses:Food     1.00 USD\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	later := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	st.submitForm()
	a.Render()
	if !st.form.stale || !strings.Contains(st.form.err, "changed on disk") {
		t.Fatalf("submit against a changed file: err = %q, stale = %v", st.form.err, st.form.stale)
	}
	// Exact label: the header shows the ledger's path, which is under a
	// directory named after this test.
	a.TapLabel("Reload")
	a.Render()
	if st.form.err != "" || st.form.stale {
		t.Fatalf("after Reload: err = %q, stale = %v", st.form.err, st.form.stale)
	}
	if st.form.payee != "Cafe" || st.form.amount != "4.50" {
		t.Errorf("Reload lost the typed entry: payee %q amount %q", st.form.payee, st.form.amount)
	}
	if a.HasLabel("Reload") {
		t.Error("the Reload button is still offered after reloading")
	}

	st.submitForm()
	a.Render()
	if st.form.err != "" || !strings.Contains(st.form.result, "saved") {
		t.Fatalf("save after Reload: err = %q, result = %q", st.form.err, st.form.result)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"Elsewhere"`) || !strings.Contains(string(raw), `"Cafe" "Coffee"`) {
		t.Errorf("the file should hold both the external edit and the new entry:\n%s", raw)
	}
}
