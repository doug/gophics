package ui

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

func galleryApp(t *testing.T, root any) *apptest.App {
	t.Helper()
	return apptest.New(t, root, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
	}))
}

// Each of the three sections the old feed was split into must stand on its own:
// one subject, visible without hunting for it. Splitting them is only an
// improvement if each still demonstrates the thing it is named after.
func TestSplitSectionsEachStandAlone(t *testing.T) {
	t.Run("pull to refresh", func(t *testing.T) {
		a := galleryApp(t, refreshSection{})
		a.AssertText("Drag down")
		a.AssertText("Refreshed 0 times")
	})

	t.Run("swipe to dismiss", func(t *testing.T) {
		a := galleryApp(t, dismissSection{})
		a.AssertText("Swipe a row")
		a.AssertText("Removed 0 of 5")
	})
}

// Swiping a row must remove that row and no other. This drives the real
// gesture through the mounted section rather than calling remove() directly,
// because the wiring under test is Dismissible + WithKey: without a stable key
// the reconciler matches rows by position and the wrong one disappears.
func TestSwipingARowRemovesThatRow(t *testing.T) {
	a := galleryApp(t, dismissSection{})

	a.AssertText("Removed 0 of 5")

	// The first row's title, so we can check it is the one that goes.
	seed := &dismissState{}
	seed.cards = makeCards(5, 40)
	first := seed.cards[0].title
	second := seed.cards[1].title

	n := a.NodeContaining(first)
	if n == nil {
		t.Fatalf("first row %q not found; labels=%v", first, a.Labels())
	}

	// Swipe it well past the dismissal threshold.
	y := (n.Rect.Min.Y + n.Rect.Max.Y) / 2
	a.Drag(geom.Pt{X: n.Rect.Min.X + 20, Y: y}, geom.Pt{X: n.Rect.Min.X + 400, Y: y})
	for range 60 {
		a.Step(1.0 / 60)
	}

	if a.HasText(first) {
		t.Errorf("swiped row %q is still present", first)
	}
	if !a.HasText(second) {
		t.Errorf("row %q disappeared instead of the one that was swiped", second)
	}
	if !a.HasText("Removed 1 of 5") {
		t.Errorf("counter did not advance; labels=%v", a.Labels())
	}
}

// Pull-to-refresh replaces the list's contents. This covers what
// TestFeedPullToRefresh used to, against the section that now owns the
// behaviour.
func TestPullToRefreshReplacesTheList(t *testing.T) {
	a := galleryApp(t, refreshSection{})

	seed := &refreshState{}
	seed.cards = makeCards(6, 0)
	firstTitle := seed.cards[0].title

	if !a.HasText(firstTitle) {
		t.Fatalf("expected the initial list to show %q; labels=%v", firstTitle, a.Labels())
	}
	a.AssertText("Refreshed 0 times")

	// Drag down from the top of the list, past the refresh threshold.
	a.Move(geom.Pt{X: 210, Y: 300})
	a.DragTo(geom.Pt{X: 210, Y: 300}, geom.Pt{X: 210, Y: 600})
	a.Release(geom.Pt{X: 210, Y: 600})
	for range 90 {
		a.Step(1.0 / 60)
	}

	if !a.HasText("Refreshed once") {
		t.Errorf("the list did not refresh; labels=%v", a.Labels())
	}
	if a.HasText(firstTitle) {
		t.Errorf("after refreshing, the list still leads with %q", firstTitle)
	}
}
