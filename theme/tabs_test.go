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

// TestTabsOnChange proves a Tabs bar reports the index of the tapped tab (and
// not the already-active one).
func TestTabsOnChange(t *testing.T) {
	var got int = -1
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{theme.Tabs{
			Tabs:     []string{"All", "Unread", "Flagged", "Sent"},
			Selected: 0,
			OnChange: func(i int) { got = i },
		}},
	}
	const w, h = 400, 120
	hd, err := app.NewHeadless(root, app.Config{
		Size:         geom.Size{W: w, H: h},
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	hd.Resize(geom.Size{W: w, H: h})
	hd.Render() // paint once so the bar captures its width

	// Four even tabs across 400px → tab 3 spans [300,400). Tap its center; y=20
	// lands inside the 40px-tall label row at the top.
	tap := geom.Pt{X: 350, Y: 20}
	hd.Press(tap)
	hd.Release(tap)
	if got != 3 {
		t.Fatalf("tap on tab 3 reported %d", got)
	}

	// Tapping the active tab reports nothing new.
	got = -1
	active := geom.Pt{X: 50, Y: 20}
	hd.Press(active)
	hd.Release(active)
	if got != -1 {
		t.Fatalf("tapping the active tab fired OnChange (%d)", got)
	}
}

// TestTabsEmpty proves a zero-tab bar builds without panicking.
func TestTabsEmpty(t *testing.T) {
	root := widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children:   []widget.Widget{theme.Tabs{}},
	}
	hd, err := app.NewHeadless(root, app.Config{Size: geom.Size{W: 120, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	hd.Resize(geom.Size{W: 120, H: 60})
	hd.Render()
}
