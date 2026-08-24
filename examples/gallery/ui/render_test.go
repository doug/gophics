package ui

import (
	"image"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

func cfg() app.Config {
	return app.Config{
		Size:         geom.Size{W: 420, H: 760},
		Background:   theme.Light().Bg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}
}

// startHome boots the catalog and returns the headless app, the Navigator
// handle from the home screen, and the root state (for driving the theme).
func startHome(t *testing.T) (*app.Headless, widget.Nav, *galleryState) {
	t.Helper()
	var nav widget.Nav
	var root *galleryState
	rootHook = func(s *galleryState) { root = s }
	homeHook = func(n widget.Nav) { nav = n }
	defer func() { rootHook, homeHook = nil, nil }()

	h, err := app.NewHeadless(Gallery{}, cfg(), 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if root == nil {
		t.Fatal("root state hook did not fire")
	}
	return h, nav, root
}

func settle(h *app.Headless) {
	for i := 0; i < 120 && h.Step(0.016); i++ {
		h.Render()
	}
	h.Render()
}

// lum returns the red channel (0..1) of the logical pixel at (lx,ly), scale 2.
func lum(img image.Image, lx, ly int) float64 {
	r, _, _, _ := img.At(lx*2, ly*2).RGBA()
	return float64(r) / 0xffff
}

// colorful reports whether the pixel at logical (lx,ly) is a vivid swatch color
// (scale 2), distinguishing a gradient header from flat chrome.
func colorful(img image.Image, lx, ly int) bool {
	r, g, b, _ := img.At(lx*2, ly*2).RGBA()
	max, min := maxu(r, g, b), minu(r, g, b)
	return max > 0x6000 && max-min > 0x2000
}

func maxu(vs ...uint32) uint32 {
	m := uint32(0)
	for _, v := range vs {
		if v > m {
			m = v
		}
	}
	return m
}

func minu(vs ...uint32) uint32 {
	m := ^uint32(0)
	for _, v := range vs {
		if v < m {
			m = v
		}
	}
	return m
}

// TestHomeThemeSwitch checks the home renders and that flipping the root theme
// mode re-provides the theme, repainting the background from light to dark.
func TestHomeThemeSwitch(t *testing.T) {
	h, _, root := startHome(t)

	// The Light background is a warm off-white — bright.
	if got := lum(h.Render(), 405, 8); got < 0.7 {
		t.Fatalf("light background should be bright, got r=%.2f", got)
	}

	// Drive the switcher via root state; Build re-provides theme.Dark().
	root.SetState(func() { root.mode = modeDark })
	settle(h)
	if got := lum(h.Render(), 405, 8); got > 0.3 {
		t.Fatalf("dark background should be dark, got r=%.2f", got)
	}
}

// TestSectionsSmoke pushes each catalog section, renders it, and pops back — so
// every section builds and lays out without panicking.
func TestSectionsSmoke(t *testing.T) {
	h, nav, _ := startHome(t)

	// Guard that the catalog still carries the newer sections (Selection,
	// Pickers) alongside the originals, so a registry regression is caught here
	// rather than silently shrinking the tour.
	have := map[string]bool{}
	for _, sec := range sections() {
		have[sec.title] = true
	}
	for _, want := range []string{"Selection", "Pickers", "Dialogs & menus"} {
		if !have[want] {
			t.Fatalf("catalog is missing the %q section", want)
		}
	}

	for i, sec := range sections() {
		nav.Push(sec.page())
		settle(h)
		if d := nav.Depth(); d != 2 {
			t.Fatalf("section %d (%s): depth = %d, want 2", i, sec.title, d)
		}
		h.Render()
		nav.Pop()
		settle(h)
		if d := nav.Depth(); d != 1 {
			t.Fatalf("section %d (%s): after pop depth = %d, want 1", i, sec.title, d)
		}
	}
}

// openFeed pushes the Navigation & gestures feed page and returns its state.
func openFeed(t *testing.T, h *app.Headless, nav widget.Nav) *feedState {
	t.Helper()
	var st *feedState
	feedHook = func(s *feedState) { st = s }
	defer func() { feedHook = nil }()
	nav.Push(feedPage{})
	settle(h)
	if st == nil {
		t.Fatal("feed state hook did not fire")
	}
	return st
}

func TestFeedHeroNavigation(t *testing.T) {
	h, nav, _ := startHome(t)
	openFeed(t, h, nav)

	// On the feed the header area (center column) is chrome, not a swatch.
	if colorful(h.Render(), 210, 80) {
		t.Fatal("feed header area should not be a swatch color")
	}

	// Tap a card → push detail; the hero swatch flies into a full-bleed header.
	h.Tap(geom.Pt{X: 210, Y: 220})
	settle(h)
	if !colorful(h.Render(), 210, 80) {
		t.Fatal("detail should show a full-bleed gradient header after the flight")
	}

	// Tap the header → pop back to the feed.
	h.Tap(geom.Pt{X: 210, Y: 80})
	settle(h)
	if colorful(h.Render(), 210, 80) {
		t.Fatal("back on the feed the header area should be chrome again")
	}
}

// Pull-to-refresh and swipe-to-dismiss moved out of the feed into their own
// sections, so the feed no longer demonstrates them; those behaviours are
// covered against the sections that now own them, in gestures_split_test.go.
// The feed keeps what only it can show: a push transition with a Hero.

func TestDetailSelectableBody(t *testing.T) {
	h, nav, _ := startHome(t)
	openFeed(t, h, nav)

	// Open a known card's detail directly, so the selection is deterministic.
	c := makeCards(12, 0)[0]
	nav.Push(detailPage{card: c})
	settle(h)

	// Drag horizontally across the body paragraph and copy. The body sits below
	// the 200px header, the back button, title and byline; start past the
	// left-edge back-swipe strip (~22px).
	h.DragTo(geom.Pt{X: 40, Y: 360}, geom.Pt{X: 380, Y: 360})
	h.Release(geom.Pt{X: 380, Y: 360})
	h.KeyMod(shell.KeyC, shell.ModSuper)

	got := h.Owner().Clipboard.(*app.MemClipboard).S
	if got == "" {
		t.Fatal("dragging across the detail body selected nothing")
	}
	if !strings.Contains(bodyFor(c), got) {
		t.Fatalf("selection %q is not a substring of the body", got)
	}
	if len(got) < 8 {
		t.Fatalf("selection %q suspiciously short for a full-width drag", got)
	}
}
