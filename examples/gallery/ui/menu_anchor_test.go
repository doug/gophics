package ui

import (
	"math"
	"testing"
)

// The anchored menu hangs from the button that raised it. It used to open at
// a hard-coded window point, which was only near the button at one window
// size and scroll position — under the wide-window centring it was far off.
func TestMenuOpensUnderItsButton(t *testing.T) {
	a := galleryApp(t, dialogsSection{})
	label := a.NodeContaining("Show menu")
	if label == nil {
		t.Fatalf("no Show menu button; labels=%v", a.Labels())
	}
	button := label.Rect

	a.TapText("Show menu")
	a.Render()
	item := a.NodeContaining("Rename")
	if item == nil {
		t.Fatalf("menu did not open; labels=%v", a.Labels())
	}
	// Both labels sit inside the same 14px horizontal padding, so a menu
	// anchored at the button's left edge lines its text up with the label.
	if dx := math.Abs(float64(item.Rect.Min.X - button.Min.X)); dx > 4 {
		t.Errorf("menu text at x=%.0f, button label at x=%.0f — the menu is not anchored at the button", item.Rect.Min.X, button.Min.X)
	}
	if item.Rect.Min.Y <= button.Max.Y || item.Rect.Min.Y-button.Max.Y > 40 {
		t.Errorf("menu text at y=%.0f, button label bottom at y=%.0f — the menu should open just under the button", item.Rect.Min.Y, button.Max.Y)
	}
}
