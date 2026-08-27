//go:build gophics_gpu

package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// The device counters sit on a path that actually runs.
//
// This exists because the previous attempt to explain the device's spikes used
// the glyph atlas upload counter, which reported zero on every frame — not
// because nothing was uploaded but because that counter lives on a text mode
// nothing here selects. The probe could not have found anything, and looked
// like evidence while it did so.
//
// So before trusting these numbers: render a real GPU frame and require the
// counters to have moved. A counter that cannot move is worse than no counter.
func TestDeviceStatsCountARealFrame(t *testing.T) {
	root := widget.Fill{Color: paint.RGB(0.2, 0.4, 0.6),
		Child: widget.Sized{W: 100, H: 100, Child: widget.Decorated{Color: paint.RGB(1, 1, 1)}}}
	h, err := NewHeadless(root, Config{
		Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	before, beforePipes := wgpu.DeviceStats()
	if h.RenderGPU() == nil {
		t.Skip("no GPU adapter available")
	}
	after, afterPipes := wgpu.DeviceStats()

	if after == before && afterPipes == beforePipes {
		t.Error("a GPU frame created neither a texture nor a pipeline: the counters " +
			"are not on the path this renderer takes, and would report zero forever")
	}
}
