//go:build gophics_gpu

package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// fillApp is a full-window fill whose colour is read at build time, so the
// recorded scene is a plain rect op that Diff compares by value — a Canvas
// closure would count as changed every frame and hide the bug below.
type fillApp struct{ col *paint.Color }

func (a fillApp) Build(widget.Ctx) widget.Widget { return widget.Decorated{Color: *a.col} }

// A CPU Render after a GPU RenderGPU must repaint. RenderGPU recorded through
// the CPU variant of RecordScene, which swapped prev to a frame only the GPU
// had drawn without marking the CPU surface stale; the Render that followed
// diffed an unchanged scene, saw nothing to do, and returned the image cached
// from before the state change — in the equivalence harness, exactly where
// the two are compared.
func TestRenderRepaintsAfterRenderGPU(t *testing.T) {
	col := paint.RGB(1, 0, 0)
	h, err := NewHeadless(fillApp{&col}, Config{Size: geom.Size{W: 40, H: 40}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := h.Render().At(20, 20).RGBA(); r < 0xf000 {
		t.Fatalf("first render: red = %#x, want red", r)
	}
	col = paint.RGB(0, 0, 1)
	h.Owner().RebuildAll()
	if h.RenderGPU() == nil {
		t.Skip("no GPU adapter: cannot exercise the GPU→CPU handoff (capability, not a defect)")
	}
	img := h.Render()
	r, _, b, _ := img.At(20, 20).RGBA()
	if b < 0xf000 || r > 0x1000 {
		t.Errorf("Render after RenderGPU: red=%#x blue=%#x (skipped=%v); want the new blue, not the cached red",
			r, b, h.Skipped())
	}
}
