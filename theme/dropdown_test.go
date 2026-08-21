package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// ddApp hosts a Dropdown under a theme provider and records selections, so the
// test can drive open → pick end-to-end through the real input/overlay stack.
type ddApp struct{ hook func(*ddState) }

func (a ddApp) CreateState() widget.State { s := &ddState{}; s.hook = a.hook; return s }

type ddState struct {
	widget.StateBase[ddApp]
	hook     func(*ddState)
	selected int
	picked   int // count of OnChange calls
}

func (s *ddState) Init(widget.Ctx) { s.hook(s); s.selected = -1 }

func (s *ddState) Build(widget.Ctx) widget.Widget {
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Fill{
		Color: theme.Light().Bg,
		Child: widget.Padding{All: 20, Child: widget.Align{X: 0, Y: 0, Child: widget.Sized{W: 220, Child: theme.Dropdown{
			Options:     []string{"Small", "Medium", "Large"},
			Selected:    s.selected,
			Placeholder: "Pick a size",
			OnChange: func(i int) {
				s.SetState(func() { s.selected = i; s.picked++ })
			},
		}}}},
	}}
}

func ddHarness(t *testing.T) (*apptest.App, *ddState) {
	t.Helper()
	var st *ddState
	a := apptest.New(t, ddApp{hook: func(s *ddState) { st = s }},
		apptest.WithConfig(app.Config{
			Size: geom.Size{W: 400, H: 400}, Font: goregular.TTF,
		}),
		apptest.Scale(2),
	)
	a.Render()
	return a, st
}

// TestDropdownConstruct is the lightweight logic check: at rest it shows the
// placeholder (Selected out of range) and none of the options.
func TestDropdownConstruct(t *testing.T) {
	h, st := ddHarness(t)
	if st.selected != -1 {
		t.Fatalf("initial selection = %d, want -1", st.selected)
	}
	if !h.HasText("Pick a size") {
		t.Fatal("placeholder not shown at rest")
	}
	if h.HasText("Medium") {
		t.Fatal("options should not be visible before opening")
	}
}

// TestDropdownOpensAndSelects drives the full flow: tapping the closed control
// opens the popup (options become visible), picking one fires OnChange, closes
// the popup, and the control now shows the chosen value.
func TestDropdownOpensAndSelects(t *testing.T) {
	h, st := ddHarness(t)

	h.TapText("Pick a size")
	h.Render()
	if !h.HasText("Medium") {
		t.Fatal("popup did not open (options not visible)")
	}

	h.TapText("Medium")
	h.Render()

	if st.picked != 1 {
		t.Fatalf("OnChange fired %d times, want 1", st.picked)
	}
	if st.selected != 1 {
		t.Fatalf("selected index = %d, want 1 (Medium)", st.selected)
	}
	// Popup closed: the other options are gone again.
	if h.HasText("Large") {
		t.Fatal("popup did not close after selection")
	}
	if !h.HasText("Medium") {
		t.Fatal("control should now show the selected value")
	}
}

// TestDropdownOutsideTapDismisses opens the popup then taps the scrim (a corner
// clear of the menu); the popup closes without firing OnChange.
func TestDropdownOutsideTapDismisses(t *testing.T) {
	h, st := ddHarness(t)
	h.TapText("Pick a size")
	h.Render()
	if !h.HasText("Small") {
		t.Fatal("popup did not open")
	}
	// Bottom-right corner: on the scrim, clear of the top-left-anchored menu.
	h.Tap(geom.Pt{X: 390, Y: 390})
	h.Render()
	if h.HasText("Small") {
		t.Fatal("outside tap did not dismiss the popup")
	}
	if st.picked != 0 {
		t.Fatalf("OnChange should not fire on dismiss; fired %d", st.picked)
	}
}
