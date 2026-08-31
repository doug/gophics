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
	before := wgpu.DeviceStats()
	if h.RenderGPU() == nil {
		t.Skip("no GPU adapter available")
	}
	made := wgpu.DeviceStats().Sub(before)

	if made.Textures == 0 && made.Pipelines == 0 {
		t.Error("a GPU frame created neither a texture nor a pipeline: the counters " +
			"are not on the path this renderer takes, and would report zero forever")
	}
	// The same proof for the two counters added for F1. These are the ones the
	// plan rests on — tier 2b is claimed to create six GPU objects per path per
	// frame — so a counter that sits off the renderer's path would make that
	// finding unfalsifiable while looking like evidence.
	if made.Buffers == 0 {
		t.Error("a GPU frame created no buffer: CreateBuffer is not the choke point " +
			"it is documented to be, and the F1 measurement would read zero forever")
	}
	if made.BindGroups == 0 {
		t.Error("a GPU frame created no bind group: CreateBindGroup is not on the " +
			"renderer's path, and the F1 measurement would read zero forever")
	}
	t.Logf("one GPU frame made %d buffers, %d bind groups, %d textures, %d pipelines",
		made.Buffers, made.BindGroups, made.Textures, made.Pipelines)

	// Same proof for the transfer counters. They exist to make Phase E's own
	// success criterion — uploaded bytes falling to the damage rect's area —
	// measurable at all, and a byte counter sitting off the path data actually
	// takes would read zero forever while looking like evidence.
	x := wgpu.TransferStats()
	if x.Uploaded() == 0 {
		t.Error("a GPU frame uploaded no bytes: WriteBuffer/WriteTexture are not " +
			"where data reaches this device, and the transfer counters are blind")
	}
	t.Logf("one GPU frame moved %d bytes up (%d buffer, %d texture), %d back",
		x.Uploaded(), x.BufferBytes, x.TextureBytes, x.ReadbackBytes)
}
