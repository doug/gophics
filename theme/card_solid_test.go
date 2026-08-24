package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// A solid card records no backdrop blur.
//
// The blur is the one op whose cost is not its own — it re-reads everything
// drawn behind it — so the count is worth pinning rather than the frame time,
// which is machine-dependent and flaky. Over the gallery's charts page,
// removing five of these took the median frame from 16.5ms to 3.9 and the
// worst from 43 to 5.7.
func blursIn(t *testing.T, w widget.Widget) int {
	t.Helper()
	h, err := app.NewHeadless(
		widget.Provide[theme.Theme]{Value: theme.Glass(), Child: w},
		app.Config{Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h.Scene().BackdropBlurs()
}

func TestSolidCardsSkipTheBackdropBlur(t *testing.T) {
	body := widget.Sized{W: 200, H: 100}

	frosted := blursIn(t, theme.Card{Child: body})
	if frosted != 1 {
		t.Fatalf("a frosted glass card recorded %d backdrop blurs, want 1 — "+
			"if this is 0 the comparison below proves nothing", frosted)
	}

	solid := blursIn(t, theme.Card{Solid: true, Child: body})
	if solid != 0 {
		t.Errorf("a solid card recorded %d backdrop blurs, want none", solid)
	}
}
