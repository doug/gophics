package app

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

type inspectorApp struct{}

func (inspectorApp) CreateState() widget.State { return &inspectorState{} }

type inspectorState struct{ widget.StateBase[inspectorApp] }

func (s *inspectorState) Build(widget.Ctx) widget.Widget {
	return widget.Center(widget.Sized{W: 100, H: 40})
}

func inspHarness(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(inspectorApp{},
		Config{Size: geom.Size{W: 300, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func TestDeepestAtFindsLeaf(t *testing.T) {
	h := inspHarness(t)
	root := h.core.Owner.RootBox()

	// The 100×40 box is centered in 300×200: x∈[100,200], y∈[80,120].
	_, rect, ok := layout.DeepestAt(root, geom.Pt{X: 150, Y: 100})
	if !ok {
		t.Fatal("no box found under the centered point")
	}
	if rect.Dx() != 100 || rect.Dy() != 40 {
		t.Fatalf("deepest box = %.0f×%.0f at %v, want 100×40", rect.Dx(), rect.Dy(), rect.Min)
	}

	// An empty corner still resolves to some ancestor filling that area.
	_, corner, ok := layout.DeepestAt(root, geom.Pt{X: 5, Y: 5})
	if !ok {
		t.Fatal("no box found at the corner")
	}
	if corner.Dx()*corner.Dy() <= rect.Dx()*rect.Dy() {
		t.Fatalf("corner box (%v) should be larger than the centered leaf", corner)
	}
}

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInspectorOverlayDraws(t *testing.T) {
	h := inspHarness(t)
	plain := encode(t, h.Render())

	h.core.SetInspect(true)
	h.Move(geom.Pt{X: 150, Y: 100}) // hover the centered box
	withOverlay := encode(t, h.Render())

	if bytes.Equal(plain, withOverlay) {
		t.Fatal("inspector overlay did not change the rendered frame")
	}

	// Toggling off restores the plain frame.
	h.core.SetInspect(false)
	h.Move(geom.Pt{X: 150, Y: 100})
	off := encode(t, h.Render())
	if !bytes.Equal(plain, off) {
		t.Fatal("disabling the inspector did not restore the plain frame")
	}
}
