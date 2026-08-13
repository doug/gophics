package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"

	"github.com/dougfritz/tally/bean"
	"github.com/dougfritz/tally/book"
)

// harness mounts the app headlessly and returns it with its root state.
func harness(t *testing.T) (*app.Headless, *state) {
	t.Helper()
	var st *state
	stateHook = func(s *state) { st = s }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(Tally{}, app.Config{
		Size: geom.Size{W: 1040, H: 680}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("root state never mounted")
	}
	return h, st
}

// TestBalancesLoad checks the embedded ledger opens and the balance tree builds.
func TestBalancesLoad(t *testing.T) {
	h, s := harness(t)
	if s.err != nil {
		t.Fatalf("ledger failed to load: %v", s.err)
	}
	if s.tree == nil || len(s.tree.Roots) == 0 {
		t.Fatal("no balance tree")
	}
	if out := os.Getenv("TALLY_BALANCES_OUT"); out != "" {
		dump(t, h, out)
	}
}

// TestRegisterDrilldown opens a known account's register and checks the table has
// rows with a running balance — the P1b slice, end to end through the UI state.
func TestRegisterDrilldown(t *testing.T) {
	h, s := harness(t)

	s.open("Assets:US:BofA:Checking")
	h.Render()

	if s.account != "Assets:US:BofA:Checking" {
		t.Fatalf("did not drill in; account = %q", s.account)
	}
	if len(s.entries) == 0 {
		t.Fatal("register has no entries")
	}
	if s.currency == "" {
		t.Error("register has no currency")
	}
	last := s.entries[len(s.entries)-1]
	if last.Balance.IsZero() && last.Amount.IsZero() {
		t.Error("last register row has neither amount nor balance")
	}
	if out := os.Getenv("TALLY_REGISTER_OUT"); out != "" {
		dump(t, h, out)
	}

	// The filter narrows the visible rows without touching the loaded set.
	all := len(s.visibleEntries())
	s.filter = "Kin Soy"
	if got := len(s.visibleEntries()); got >= all {
		t.Errorf("filter did not narrow rows: %d of %d", got, all)
	}
	s.filter = "zzz-no-such-payee"
	if got := len(s.visibleEntries()); got != 0 {
		t.Errorf("nonsense filter matched %d rows", got)
	}

	// Going back returns to the balances overview.
	s.filter = ""
	s.back()
	h.Render()
	if s.account != "" || s.entries != nil {
		t.Error("back() did not return to balances")
	}
}

func dump(t *testing.T, h *app.Headless, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.Render()); err != nil {
		t.Fatal(err)
	}
}

// TestAddFormWritesThroughUI drives the add-transaction panel the way a user
// would — open it, fill it in, submit — and checks the entry reaches the file and
// every view refreshes. This is the local-first loop end to end: read the ledger,
// add to it, save it back.
func TestAddFormWritesThroughUI(t *testing.T) {
	// A real file, so the write path is exercised rather than the in-memory one.
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.beancount")
	body := `2021-01-01 open Assets:Cash USD
2021-01-01 open Expenses:Food USD

2021-01-10 * "Grocer" "Weekly shop"
  Assets:Cash      -50.00 USD
  Expenses:Food     50.00 USD
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	h, s := harness(t)
	b, err := book.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.book, s.loaded, s.prefsChecked = b, true, true
	s.tree, _ = b.Tree()
	s.baseCurrency = b.MainCurrency()
	h.Render()

	s.toggleForm()
	h.Render()
	if !s.form.open {
		t.Fatal("the form did not open")
	}
	if s.form.date == "" {
		t.Error("the form should be seeded with today's date")
	}

	s.form.date = "2021-01-20"
	s.form.payee, s.form.narration = "Cafe", "Coffee"
	s.form.from, s.form.to = "Assets:Cash", "Expenses:Food"
	s.form.amount = "4.50"
	s.submitForm()
	h.Render()

	if s.form.err != "" {
		t.Fatalf("submit reported an error: %s", s.form.err)
	}
	if !strings.Contains(s.form.result, "saved") {
		t.Errorf("result = %q, want a saved confirmation", s.form.result)
	}

	// The file on disk has it, and the user's original entry survived.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"Cafe" "Coffee"`) {
		t.Error("the entry did not reach the file")
	}
	if !strings.Contains(string(raw), "Weekly shop") {
		t.Error("saving clobbered the existing entry")
	}

	// The balance tree the UI draws from reflects the new total.
	var food string
	s.tree.Walk(func(n *bean.Node) {
		if n.Account == "Expenses:Food" {
			food = n.Balance.Get("USD").String()
		}
	})
	if food != "54.5" {
		t.Errorf("tree shows Expenses:Food = %s, want 54.5", food)
	}

	// A bad amount is rejected without touching the file.
	s.form.amount = "not a number"
	s.submitForm()
	if s.form.err == "" {
		t.Error("a non-numeric amount should be rejected")
	}
	after, _ := os.ReadFile(path)
	if len(after) != len(raw) {
		t.Error("a rejected entry modified the file")
	}
}
