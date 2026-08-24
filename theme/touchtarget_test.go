package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Every tappable control has to be at least as tall as a fingertip.
//
// theme.MinTouchTarget is the constant both Apple and Google land on, and the
// switch already grew its hit area to meet it. Checkbox and Radio did not: a
// labeled row is 20pt tall, which is the difference between "toggles when I tap
// it" and "sometimes toggles". Width is not checked because a label already
// makes these wide, and pinning it would clip the label.
func TestTappableControlsMeetTheTouchTarget(t *testing.T) {
	cases := []struct {
		name   string
		widget widget.Widget
		role   layout.Role
	}{
		{"Checkbox", theme.Checkbox{Label: "Mushroom", OnChange: func(bool) {}}, layout.RoleCheckbox},
		{"Radio", theme.Radio{Label: "Free", OnSelect: func() {}}, layout.RoleRadio},
		{"Switch", theme.Switch{Label: "Notifications", OnChange: func(bool) {}}, layout.RoleSwitch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// In a Column the control takes its intrinsic height. Mounted as
			// the root it would stretch to fill the window and pass whatever
			// its real height was, which is a test that cannot fail.
			a := apptest.New(t, widget.Column(tc.widget, widget.Sized{H: 100}),
				apptest.WithConfig(app.Config{
					Size: geom.Size{W: 320, H: 200}, Font: goregular.TTF,
				}))
			n := a.Role(tc.role)
			if n == nil {
				t.Fatalf("no %s in the tree; labels=%v", tc.name, a.Labels())
			}
			if h := n.Rect.Dy(); h < theme.MinTouchTarget {
				t.Errorf("%s hit area is %.0fpt tall, want at least %d — "+
					"a target shorter than a fingertip misses",
					tc.name, h, theme.MinTouchTarget)
			}
		})
	}
}

// A switch is drawn, not written, so without a Label a screen reader can only
// say "on" — true, and useless. Its own documentation says so.
func TestSwitchCarriesItsLabel(t *testing.T) {
	a := apptest.New(t, theme.Switch{Label: "Enable notifications", OnChange: func(bool) {}},
		apptest.WithConfig(app.Config{Size: geom.Size{W: 320, H: 200}, Font: goregular.TTF}))

	n := a.Role(layout.RoleSwitch)
	if n == nil {
		t.Fatal("no switch in the tree")
	}
	if n.Label != "Enable notifications" {
		t.Errorf("switch label = %q, want it to name what it controls", n.Label)
	}
}

// A stack of checkboxes must line up down their boxes.
//
// touchTargetH raised these to a fingertip's height with widget.Center, which
// centres on both axes — so given a column wider than the control, which is
// every form, each row drifted to the middle and a group came out ragged
// instead of aligned. The height assertion above passed throughout: it is the
// axis nobody was looking at.
func TestLabeledControlsAlignToTheLeadingEdge(t *testing.T) {
	const width = 320
	for _, tc := range []struct {
		name   string
		widget widget.Widget
	}{
		{"Checkbox", theme.Checkbox{Label: "Mushroom", OnChange: func(bool) {}}},
		{"Radio", theme.Radio{Label: "Free", OnSelect: func() {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Measured in pixels, not from the semantic tree. The node these
			// controls publish is the hit area, which spans the row whatever
			// the content inside it does — so a rect assertion passes while
			// the thing a person sees is centred. The leftmost painted pixel
			// is the ragged edge itself.
			a := apptest.New(t, widget.Column(
				widget.Sized{W: width, Child: tc.widget},
				widget.Sized{H: 100},
			), apptest.WithConfig(app.Config{
				Size: geom.Size{W: width, H: 200}, Font: goregular.TTF,
			}))

			img := a.Render()
			b := img.Bounds()
			bg := img.At(b.Max.X-2, b.Max.Y-2) // a corner the control never reaches
			leftmost := b.Max.X
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < leftmost; x++ {
					if img.At(x, y) != bg {
						leftmost = x
						break
					}
				}
			}
			// Scaled: the window is in points, the framebuffer in pixels.
			limit := b.Dx() / 8
			if leftmost > limit {
				t.Errorf("%s paints nothing until x=%d of %d, want it to start near the "+
					"leading edge — a centred control makes a column of them ragged",
					tc.name, leftmost, b.Dx())
			}
		})
	}
}
