package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// secondaryField builds a single field in an overlay host, the shape every
// edit-menu test needs.
func secondaryField(t *testing.T, value string) *app.Headless {
	t.Helper()
	root := widget.OverlayHost{Child: widget.Center(widget.Sized{
		W: 240, H: 44,
		Child: widget.TextField{Value: value, OnChange: func(string) {}},
	})}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 320, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

// A right-click raises the edit menu on the first click, not the second.
//
// On the web it took two: the first right-click on a field that did not
// already have focus put the menu up and something took it straight back
// down, so the user saw nothing happen and clicked again. Every other
// platform raises the menu on the first press of the secondary button, and a
// context menu that needs two right-clicks reads as broken.
func TestSecondaryTapRaisesTheMenuOnTheFirstClick(t *testing.T) {
	h := secondaryField(t, "hello world")
	h.Clipboard().S = "something pasteable"

	at := geom.Pt{X: 160, Y: 100}
	h.SecondaryTap(at)
	h.Render()

	if !hasLabel(h, "Paste") {
		t.Fatalf("no edit menu after one right-click on an unfocused field; labels: %v", labels(h))
	}
}

// And it stays up: a second right-click in the same place rebuilds the menu
// rather than toggling it away.
func TestSecondaryTapKeepsTheMenuOnASecondClick(t *testing.T) {
	h := secondaryField(t, "hello world")
	h.Clipboard().S = "something pasteable"

	at := geom.Pt{X: 160, Y: 100}
	h.SecondaryTap(at)
	h.Render()
	h.SecondaryTap(at)
	h.Render()

	if !hasLabel(h, "Paste") {
		t.Fatalf("the menu went away on a second right-click; labels: %v", labels(h))
	}
}
