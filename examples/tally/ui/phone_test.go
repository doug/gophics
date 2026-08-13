package ui

import (
	"image"
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

// phoneHarness mounts the app at an iPhone's logical size and scale.
func phoneHarness(t *testing.T) (*app.Headless, *state) {
	t.Helper()
	var st *state
	stateHook = func(s *state) { st = s }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(App{}, app.Config{
		Size: geom.Size{W: 393, H: 852}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("root state never mounted")
	}
	return h, st
}

// rightEdgeInk reports how many pixels in the rightmost columns differ from the
// background — the signature of content running off the screen.
//
// This is the check the eye does on a screenshot, made mechanical: a layout that
// overflows paints text or a control right up to (and past) the edge, where a
// layout that fits leaves the margin clear.
func rightEdgeInk(img image.Image, marginPx int) int {
	b := img.Bounds()
	// Sample the background from a point inside the top margin, which no layout
	// paints over.
	bg := img.At(b.Min.X+2, b.Min.Y+2)
	br, bgG, bb, _ := bg.RGBA()

	ink := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Max.X - marginPx; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			// Tolerate antialiasing noise; look for genuinely different pixels.
			if diff(r, br)+diff(g, bgG)+diff(bl, bb) > 3000 {
				ink++
			}
		}
	}
	return ink
}

func diff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestPhoneLayoutDoesNotOverflow is a regression test for content running off the
// right edge on a phone. The desktop layout put three figures side by side and the
// register five columns across; at 393pt the change ("+104,621.50 USD") was
// clipped mid-number, which is worse than useless on a finance screen — a
// truncated figure still reads as a figure.
//
// Every screen is checked, because each had its own version of the bug.
func TestPhoneLayoutDoesNotOverflow(t *testing.T) {
	const margin = 6 // device pixels of margin that must stay clear

	cases := []struct {
		name  string
		setup func(*app.Headless, *state)
	}{
		{"overview", func(*app.Headless, *state) {}},
		{"balances", func(h *app.Headless, s *state) {
			s.SetState(func() { s.view = viewBalances })
		}},
		{"register", func(h *app.Headless, s *state) {
			s.open("Assets:US:BofA:Checking")
		}},
		{"add form", func(h *app.Headless, s *state) {
			s.toggleForm()
			s.form.payee, s.form.narration, s.form.amount = "Blue Bottle", "Coffee", "4.50"
		}},
	}

	for _, c := range cases {
		h, s := phoneHarness(t)
		c.setup(h, s)
		h.Render()
		img := h.Render()

		if ink := rightEdgeInk(img, margin); ink > 0 {
			t.Errorf("%s: %d pixels of content in the right margin — the layout overflows at phone width", c.name, ink)
			if out := os.Getenv("TALLY_PHONE_OUT"); out != "" {
				f, _ := os.Create(out + "-" + c.name + ".png")
				_ = png.Encode(f, img)
				f.Close()
			}
		}
	}
}

// TestPhoneChoosesNarrowLayout checks the breakpoint actually engages rather than
// the screens merely happening to fit: the register drops to three columns and the
// header shortens its button.
func TestPhoneChoosesNarrowLayout(t *testing.T) {
	h, s := phoneHarness(t)
	s.open("Assets:US:BofA:Checking")
	h.Render()

	// The account title is shortened to its last two components on a phone.
	if got := shortAccount("Assets:US:BofA:Checking", false); got != "BofA:Checking" {
		t.Errorf("narrow account title = %q, want %q", got, "BofA:Checking")
	}
	if got := shortAccount("Assets:US:BofA:Checking", true); got != "Assets:US:BofA:Checking" {
		t.Errorf("wide account title was shortened: %q", got)
	}
}

// TestDefaultAccountsPickPostingBearingOnes: the form's starting pair must be
// accounts money actually moves through, not the first parent alphabetically —
// "Assets:US:BofA" holds nothing and cannot be spent from.
func TestDefaultAccountsPickPostingBearingOnes(t *testing.T) {
	_, s := phoneHarness(t)
	from, to := s.defaultAccounts()
	if from == "" || to == "" {
		t.Fatalf("no defaults chosen: from=%q to=%q", from, to)
	}
	for _, acct := range []string{from, to} {
		if len(s.book.Currencies(acct)) == 0 {
			t.Errorf("%q has no postings of its own — it is a parent account", acct)
		}
	}
}
