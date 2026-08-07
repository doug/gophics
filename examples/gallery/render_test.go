package main

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
)

func harness(t *testing.T) (*app.Headless, *feedState) {
	t.Helper()
	var st *feedState
	feedHook = func(s *feedState) { st = s }
	defer func() { feedHook = nil }()

	h, err := app.NewHeadless(Gallery{}, app.Config{
		Size:         geom.Size{W: 420, H: 680},
		Background:   theme.Dark().Bg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.SetDarkMode(true)
	h.Render()
	if st == nil {
		t.Fatal("feed state hook did not fire")
	}
	return h, st
}

func settle(h *app.Headless) {
	for i := 0; i < 120 && h.Step(0.016); i++ {
		h.Render()
	}
	h.Render()
}

// colorful reports whether the pixel at logical (lx,ly) is a vivid swatch
// color (scale 2), distinguishing the gradient header from the dark feed.
func colorful(img image.Image, lx, ly int) bool {
	r, g, b, _ := img.At(lx*2, ly*2).RGBA()
	max, min := maxu(r, g, b), minu(r, g, b)
	return max > 0x6000 && max-min > 0x2000 // bright and saturated
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

func TestGalleryHeroNavigation(t *testing.T) {
	h, _ := harness(t)

	// Feed: the header strip (y≈100) is dark chrome, not a swatch.
	if colorful(h.Render(), 210, 100) {
		t.Fatal("feed header area should not be a swatch color")
	}

	// Tap a card → push detail; the hero swatch flies into a full-bleed header.
	h.Tap(geom.Pt{X: 210, Y: 150})
	settle(h)
	if !colorful(h.Render(), 210, 100) {
		t.Fatal("detail should show a full-bleed gradient header after the flight")
	}

	// Tap the header → pop back to the feed.
	h.Tap(geom.Pt{X: 210, Y: 100})
	settle(h)
	if colorful(h.Render(), 210, 100) {
		t.Fatal("back on the feed the header area should be dark again")
	}
}

func TestGallerySelectableBody(t *testing.T) {
	h, _ := harness(t)
	h.Tap(geom.Pt{X: 210, Y: 150}) // open detail
	settle(h)

	// Drag horizontally across the body paragraph and copy. The body sits
	// below the 200px header, back chip, title and byline. Start past the
	// left back-swipe edge strip (~22px), which reserves the very left edge.
	h.DragTo(geom.Pt{X: 40, Y: 345}, geom.Pt{X: 380, Y: 345})
	h.Release(geom.Pt{X: 380, Y: 345})
	h.KeyMod(shell.KeyC, shell.ModSuper)

	got := h.Owner().Clipboard.(*app.MemClipboard).S
	if got == "" {
		t.Fatal("dragging across the detail body selected nothing")
	}
	// A single-line horizontal selection is a contiguous slice of the body.
	body := bodyFor(makeCards(12, 0)[0])
	if !strings.Contains(body, got) {
		t.Fatalf("selection %q is not a substring of the body", got)
	}
	if len(got) < 8 {
		t.Fatalf("selection %q suspiciously short for a full-width drag", got)
	}
}

func TestGalleryPullToRefresh(t *testing.T) {
	h, st := harness(t)
	first := st.cards[0].id

	// Pull down from the top of the list, past the trigger, and release.
	h.Move(geom.Pt{X: 210, Y: 200})
	h.DragTo(geom.Pt{X: 210, Y: 200}, geom.Pt{X: 210, Y: 460})
	h.Release(geom.Pt{X: 210, Y: 460})
	settle(h)

	if st.cards[0].id == first {
		t.Fatalf("pull-to-refresh should reshuffle the feed (still id=%d)", first)
	}
}

func TestGallerySwipeDismiss(t *testing.T) {
	h, st := harness(t)
	before := len(st.cards)

	// Swipe the first card far to the right, past the threshold.
	h.DragTo(geom.Pt{X: 120, Y: 150}, geom.Pt{X: 380, Y: 150})
	h.Release(geom.Pt{X: 380, Y: 150})
	settle(h)

	if len(st.cards) != before-1 {
		t.Fatalf("swipe should remove one card: %d → %d", before, len(st.cards))
	}
}
