package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// TestTappablePressHighlight proves a Tappable row darkens its resting
// background while pressed (touch-native feedback) and recovers after release.
func TestTappablePressHighlight(t *testing.T) {
	// A near-white resting background so a press darkening reads clearly.
	base := paint.RGB(0.95, 0.95, 0.95)
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{widget.Expand(theme.Tappable{
			Background: base,
			OnTap:      func() {},
			Pad:        geom.InsetsSymmetric(12, 12),
			Child:      widget.Text{S: "Row", Size: 14, Color: paint.RGB(0, 0, 0)},
		})},
	}
	h, err := app.NewHeadless(root, app.Config{Size: geom.Size{W: 120, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: 120, H: 60})
	red := func() uint8 { r, _, _, _ := h.Render().At(6, 6).RGBA(); return uint8(r >> 8) }

	rest := red()
	h.Press(geom.Pt{X: 60, Y: 30})
	held := red()
	h.Release(geom.Pt{X: 60, Y: 30})
	for range 40 {
		h.Step(1.0 / 60)
	}
	after := red()

	if held >= rest {
		t.Fatalf("press did not darken the row: rest=%d held=%d", rest, held)
	}
	if int(rest)-int(after) > 6 {
		t.Fatalf("row did not recover after release: rest=%d after=%d", rest, after)
	}
}
