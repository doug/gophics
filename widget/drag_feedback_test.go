package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// recordingHaptic stands in for a device's tactile engine.
type recordingHaptic struct{ played []shell.HapticKind }

func (r *recordingHaptic) Play(k shell.HapticKind) { r.played = append(r.played, k) }

// A drag should be felt as well as seen: the pick-up is the moment a finger
// covering the item most needs telling that something happened, and landing
// should not feel like failing to land.
func TestDraggingPlaysHaptics(t *testing.T) {
	for _, tc := range []struct {
		name string
		to   geom.Pt
		want []shell.HapticKind
	}{
		{"onto a target", geom.Pt{X: 100, Y: acceptY}, []shell.HapticKind{shell.HapticMedium, shell.HapticSuccess}},
		{"onto nothing", geom.Pt{X: 100, Y: 190}, []shell.HapticKind{shell.HapticMedium, shell.HapticLight}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newDnD(t)
			hap := &recordingHaptic{}
			h.Owner().WireCapabilities(hapticWindow{h: hap})
			h.Render()

			h.Drag(geom.Pt{X: 100, Y: srcY}, tc.to)
			h.Render()

			if len(hap.played) != len(tc.want) {
				t.Fatalf("played %v, want %v", hap.played, tc.want)
			}
			for i := range tc.want {
				if hap.played[i] != tc.want[i] {
					t.Errorf("haptic %d was %v, want %v", i, hap.played[i], tc.want[i])
				}
			}
		})
	}
}

// Haptics are a best-effort hint, and desktop has no engine at all: a nil
// capability must be a no-op, not a crash mid-drag.
// hapticWindow is a Window whose only capability is haptics, so the fake is
// installed through the real wiring rather than by writing the Owner field —
// which is unexported now, and was always the wrong seam: going through
// WireCapabilities exercises the type-assert and the posted wrapper too.
type hapticWindow struct {
	shell.Window
	h shell.Haptic
}

func (w hapticWindow) Haptic() shell.Haptic { return w.h }

func TestDraggingWithoutHapticsIsFine(t *testing.T) {
	h, r := newDnD(t)
	h.Owner().WireCapabilities(hapticWindow{h: nil})
	h.Render()

	h.Drag(geom.Pt{X: 100, Y: srcY}, geom.Pt{X: 100, Y: acceptY})
	h.Render()

	if len(r.dropped) != 1 {
		t.Errorf("drop did not complete without a haptic engine: %v", r.dropped)
	}
}

// ghostApp is one draggable chip with a findable label, so both it and the
// copy carried in the overlay can be located in the semantic tree.
type ghostApp struct{}

func (ghostApp) Build(widget.Ctx) widget.Widget {
	return widget.Column(
		widget.Draggable{Payload: "p", Child: widget.Sized{W: 80, H: 40,
			Child: widget.Semantics{Label: "CHIP", Child: widget.Decorated{Color: paint.RGB(1, 0, 0)}}}},
		widget.Sized{H: 200},
	)
}

// While a drag is in flight the carried copy sits centred under the pointer.
//
// Padding cannot take a negative inset, so the pointer places the preview's
// top-left and it is pulled back by half its own size — which is only known
// once it has been laid out. Get that wrong on either axis and the ghost hangs
// off the finger by half its width or height, which is what it did before:
// PreviewOffset has always documented that zero centres it.
func TestTheDragGhostSitsUnderThePointer(t *testing.T) {
	h, err := app.NewHeadless(ghostApp{}, app.Config{
		Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	const chipX, chipY = 150, 20
	at := geom.Pt{X: 150, Y: 150}
	h.Press(geom.Pt{X: chipX, Y: chipY})
	h.Move(at)
	h.Render()

	var centres []geom.Pt
	for _, n := range h.Semantics() {
		if n.Label == "CHIP" {
			centres = append(centres, geom.Pt{
				X: (n.Rect.Min.X + n.Rect.Max.X) / 2,
				Y: (n.Rect.Min.Y + n.Rect.Max.Y) / 2,
			})
		}
	}
	if len(centres) != 2 {
		t.Fatalf("found %d copies of the chip, want the original and the ghost: %v", len(centres), centres)
	}

	// One is the chip left behind, the other is the ghost on the pointer.
	var ghost *geom.Pt
	for i := range centres {
		if centres[i] != (geom.Pt{X: chipX, Y: chipY}) {
			ghost = &centres[i]
		}
	}
	if ghost == nil {
		t.Fatalf("neither copy moved off the original position: %v", centres)
	}
	if *ghost != at {
		t.Errorf("the ghost is centred at %v, want it under the pointer at %v", *ghost, at)
	}
}

// The ghost keeps following after the first move.
//
// The overlay rebuilds only when an entry is pushed, removed or updated. The
// preview is pushed once, when the drag starts, and dragPreview.Build reads the
// session's position — so the position it renders is whatever was current at
// that push. Marking the *draggable* dirty on each move rebuilds the source
// widget, which is a different subtree, and never the overlay.
//
// The earlier test could not see this: it moved once, and that move was the
// one that pushed the overlay, so the first Build happened to read the
// up-to-date position. Two moves are what it takes.
func TestTheGhostKeepsUpAfterTheFirstMove(t *testing.T) {
	h, err := app.NewHeadless(ghostApp{}, app.Config{
		Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	ghostAt := func() (geom.Pt, bool) {
		var out []geom.Pt
		for _, n := range h.Semantics() {
			if n.Label == "CHIP" {
				out = append(out, geom.Pt{
					X: (n.Rect.Min.X + n.Rect.Max.X) / 2,
					Y: (n.Rect.Min.Y + n.Rect.Max.Y) / 2,
				})
			}
		}
		if len(out) != 2 {
			return geom.Pt{}, false
		}
		// The one that is not the chip left behind at its original position.
		for _, p := range out {
			if p != (geom.Pt{X: 150, Y: 20}) {
				return p, true
			}
		}
		return geom.Pt{}, false
	}

	h.Press(geom.Pt{X: 150, Y: 20})
	h.Move(geom.Pt{X: 150, Y: 120}) // starts the drag; pushes the preview
	h.Render()
	if _, ok := ghostAt(); !ok {
		t.Fatal("no ghost after the drag started")
	}

	// Carry on moving. This is the part that was never rebuilding.
	second := geom.Pt{X: 60, Y: 220}
	h.Move(second)
	h.Render()
	got, ok := ghostAt()
	if !ok {
		t.Fatal("the ghost vanished after the second move")
	}
	if got != second {
		t.Errorf("after moving to %v the ghost is at %v — it stopped following the pointer "+
			"once the overlay had been pushed", second, got)
	}
}
