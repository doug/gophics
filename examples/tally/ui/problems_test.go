package ui

import (
	"os"
	"path/filepath"
	"testing"

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
}
