package widget_test

import (
	"strings"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// cityField mounts an Autocomplete over two cities, with room for its list,
// above a second field at y 100..120; the app holds the value the way a
// controlled field expects. It returns the app and a reader for the value.
func cityField(t *testing.T, onPick func(string)) (*app.Headless, func() string) {
	t.Helper()
	value := "p"
	var m *mutableModel
	root, m0 := newMutable(func(widget.Ctx) widget.Widget {
		return widget.Column(
			widget.Sized{W: 300, H: 100, Child: widget.Align{X: 0, Y: 0, Child: widget.Autocomplete{
				Value: value, Placeholder: "City",
				Suggest: func(in string) []string {
					var out []string
					for _, c := range []string{"Paris", "Prague"} {
						if strings.HasPrefix(strings.ToLower(c), strings.ToLower(in)) {
							out = append(out, c)
						}
					}
					return out
				},
				OnChange: func(v string) { value = v; m.Rebuild() },
				OnPick:   onPick,
			}}},
			widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "other"}},
		)
	})
	m = m0
	h := headless(t, root, 320, 240)
	h.Render()
	return h, func() string { return value }
}

// The suggestion list closes when the field loses focus: clicking into
// another field used to leave it hanging under the first one, and — as a
// Modal — owning Escape from there.
func TestAutocompleteListClosesOnBlur(t *testing.T) {
	h, _ := cityField(t, nil)
	h.Tap(geom.Pt{X: 20, Y: 10})
	h.Key(shell.KeyDown) // opens the list
	h.Render()
	if !hasSemLabel(h, "Paris") {
		t.Fatal("Down did not open the list")
	}
	h.Tap(geom.Pt{X: 20, Y: 110}) // the other field: the autocomplete blurs
	h.Render()
	h.Render()
	if hasSemLabel(h, "Paris") {
		t.Error("the suggestion list is still showing after the field lost focus")
	}
	if h.Owner().TopModal() != nil {
		t.Error("the closed list still claims Escape")
	}
}

// Tapping a suggestion still picks it. The press blurs the field before the
// tap arrives — nothing focusable is under a row — and closing the list on
// that blur would unmount the row the user is about to release on.
func TestAutocompleteRowTapPicksAfterBlur(t *testing.T) {
	var picked []string
	h, value := cityField(t, func(v string) { picked = append(picked, v) })
	h.Tap(geom.Pt{X: 20, Y: 10})
	h.Key(shell.KeyDown)
	h.Render()
	r := semRect(t, h, "Paris")
	c := geom.Pt{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
	h.Press(c)
	h.Render() // a frame between press and release, as on a device
	h.Release(c)
	h.Render()
	if len(picked) != 1 || picked[0] != "Paris" {
		t.Fatalf("picked %q, want [Paris]", picked)
	}
	if value() != "Paris" {
		t.Errorf("value %q after the pick, want Paris", value())
	}
	if hasSemLabel(h, "Prague") {
		t.Error("the list is still showing after a pick")
	}
}

// A press on a row released elsewhere picks nothing, and the field it
// blurred is not coming back: the list closes then.
func TestAutocompleteRowPressReleasedOffCloses(t *testing.T) {
	h, _ := cityField(t, nil)
	h.Tap(geom.Pt{X: 20, Y: 10})
	h.Key(shell.KeyDown)
	h.Render()
	r := semRect(t, h, "Paris")
	h.Press(geom.Pt{X: 20, Y: (r.Min.Y + r.Max.Y) / 2})
	h.Render()
	h.Release(geom.Pt{X: 20, Y: 110})
	h.Render()
	h.Render()
	if hasSemLabel(h, "Paris") {
		t.Error("the list is still showing after a row press was released off it with the field blurred")
	}
}

// With the list open but empty — typed text that matches nothing — Escape
// belongs to the field. onKey used to consume it on `open` alone while the
// Modal declined on "showing", so the first Escape did nothing and the
// selection it should have collapsed survived.
func TestAutocompleteOpenEmptyPassesEscape(t *testing.T) {
	h, value := cityField(t, nil)
	h.Tap(geom.Pt{X: 20, Y: 10})
	h.Type("zz")
	h.Render()
	if value() != "pzz" {
		t.Fatalf("value %q after typing, want pzz", value())
	}
	if hasSemLabel(h, "Paris") {
		t.Fatal("the list shows with no suggestions")
	}
	h.KeyMod(shell.KeyLeft, shell.ModShift) // select the last z
	h.Key(shell.KeyEscape)                  // collapse it
	h.Clipboard().S = ""
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := h.Clipboard().S; got != "" {
		t.Errorf("Escape did not collapse the selection: copied %q", got)
	}
}
