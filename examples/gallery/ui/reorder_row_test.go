package ui_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
)

// A reorderable row's contents sit inside the row, vertically centred.
//
// The row height is fixed by ItemExtent — the list maps finger position to
// index by multiplying it — so everything inside has to fit within it. Card
// pads by 12 on its own and the row added another 8, which left the handle and
// the label with a negative height budget: they rendered below where the card
// ends rather than in the middle of it.
func TestReorderRowsCentreTheirContents(t *testing.T) {
	a := apptest.New(t, ui.Gallery{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
	}))

	const section = "Reorderable list"
	if !a.HasText(section) {
		t.Skipf("no %q section in the catalog", section)
	}
	a.Move(geom.Pt{X: 210, Y: 400})
	for i := 0; i < 20 && a.NodeContaining(section).Rect.Min.Y > 640; i++ {
		a.Scroll(geom.Pt{Y: -400})
		for s := 0; s < 40; s++ {
			a.Step(1.0 / 60)
		}
	}
	a.TapText(section)
	for s := 0; s < 40; s++ {
		a.Step(1.0 / 60)
	}

	// The row height is fixed, so the content budget is what is left after
	// the 6pt gap below the card and the card's own padding on both sides.
	// The old arrangement padded twice and left less than nothing (-2pt), so
	// the label could not fit inside the card and rendered past its bottom.
	const (
		rowH     = 44
		gap      = 6
		cardPad  = 8
		contentH = rowH - gap - 2*cardPad
	)
	planets := []string{"Mercury", "Venus", "Earth", "Mars", "Jupiter"}

	var mids []float32
	for _, planet := range planets {
		n := a.NodeContaining(planet)
		if n == nil {
			t.Fatalf("no %q row on the page", planet)
		}
		h := n.Rect.Max.Y - n.Rect.Min.Y
		if h <= 0 {
			t.Errorf("%s has no height at all (%v): the card's content budget went negative and collapsed it",
				planet, n.Rect)
		}
		if h > contentH {
			t.Errorf("%s is %.1fpt tall but its card only has %dpt to give it, so it renders outside the card",
				planet, h, contentH)
		}
		mids = append(mids, (n.Rect.Min.Y+n.Rect.Max.Y)/2)
	}

	// Evenly spaced by exactly ItemExtent: the list maps finger position to
	// index by multiplying that number, so drift here drops rows in the wrong
	// place even when the drag itself is right.
	for i := 1; i < len(mids); i++ {
		if d := mids[i] - mids[i-1]; d < rowH-0.5 || d > rowH+0.5 {
			t.Errorf("%s sits %.2fpt below %s, want %d", planets[i], d, planets[i-1], rowH)
		}
	}

}
