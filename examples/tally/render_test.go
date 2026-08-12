package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
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
