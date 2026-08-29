package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// TestButtonPressHighlight is the end-to-end proof that a themed button shows a
// pressed-down highlight on pointer-down (the touch-native feedback hover can't
// give) and eases back after release. A primary button fills the window; a fill
// pixel darkens while held and recovers once released and the fade settles.
func TestButtonPressHighlight(t *testing.T) {
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children:   []widget.Widget{widget.Expand(theme.Button{Label: "Tap", Primary: true, OnTap: func() {}})},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size:         geom.Size{W: 120, H: 80},
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: 120, H: 80})
	// Sample a fill pixel in the corner, clear of the centered label glyphs.
	red := func() uint8 { r, _, _, _ := h.Render().At(8, 8).RGBA(); return uint8(r >> 8) }

	rest := red()
	h.Press(geom.Pt{X: 60, Y: 40})
	held := red()
	h.Release(geom.Pt{X: 60, Y: 40})
	for range 40 { // let the release fade settle
		h.Step(1.0 / 60)
	}
	after := red()

	if held >= rest {
		t.Fatalf("press did not darken the button: rest=%d held=%d", rest, held)
	}
	if int(rest)-int(after) > 6 {
		t.Fatalf("button did not recover after release: rest=%d after=%d", rest, after)
	}
}
