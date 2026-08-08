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

// TestSegmentedOnChange proves a Segmented reports the index of the tapped
// segment (and not the already-selected one).
func TestSegmentedOnChange(t *testing.T) {
	var got int = -1
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{theme.Segmented{
			Options:  []string{"Day", "Week", "Month"},
			Selected: 0,
			OnChange: func(i int) { got = i },
		}},
	}
	const w, h = 300, 80
	hd, err := app.NewHeadless(root, app.Config{
		Size:         geom.Size{W: w, H: h},
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	hd.Resize(geom.Size{W: w, H: h})
	hd.Render() // paint once so the control captures its width

	// Three even segments across 300px → segment 2 spans [200,300). Tap its
	// center; y=16 lands inside the 32px-tall pill at the top.
	tap := geom.Pt{X: 250, Y: 16}
	hd.Press(tap)
	hd.Release(tap)

	if got != 2 {
		t.Fatalf("tap on segment 2 reported %d", got)
	}

	// Tapping the currently-selected segment reports nothing new.
	got = -1
	sel := geom.Pt{X: 50, Y: 16}
	hd.Press(sel)
	hd.Release(sel)
	if got != -1 {
		t.Fatalf("tapping the selected segment fired OnChange (%d)", got)
	}
}

// TestSegmentedEmpty proves a zero-option Segmented builds without panicking.
func TestSegmentedEmpty(t *testing.T) {
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children:   []widget.Widget{theme.Segmented{}},
	}
	hd, err := app.NewHeadless(root, app.Config{Size: geom.Size{W: 120, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	hd.Resize(geom.Size{W: 120, H: 60})
	hd.Render()
}
