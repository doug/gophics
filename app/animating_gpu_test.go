//go:build gophics_gpu

package app

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/wgpu"
	"github.com/doug/gophics/internal/renderref"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// What does changing one small thing cost?
//
// The GPU replays the whole display list on every changed frame. The CPU
// present path used to discard the damage rect the app had already computed —
// gg/context.go puts that miss at a 48x48 spinner updating 9KB instead of 8MB
// at 1080p — and now honours it.
//
// This measures the claim rather than repeating it — and the answer is that
// this harness cannot size it, which is worth recording where the next person
// will look.
//
// Headless renders through RenderToImage, which reads the whole surface back
// every frame. That readback is a fixed cost, it is proportional to the surface
// rather than to what changed, and damage-rect present cannot remove it. It
// swamps the thing being measured: clipping the scene to 48x48 — 1% of the area
// — came out *slower* than not clipping on a Mali (8.60 ms against 7.40), which
// is not a result about scissoring, it is a readback with noise on top.
//
// So Phase E needs an instrument this does not have. Its own "done when" asks
// for uploaded bytes to drop to the damage rect's area, and nothing counts
// uploaded bytes; frame time through a readback harness is the wrong proxy and
// will report success or failure at random. The numbers below are kept because
// they are honest about a full frame's cost on each backend, not because they
// answer what damage would save.
func TestAnimatingFrameCost(t *testing.T) {
	h, err := NewHeadless(&animScene{}, Config{
		Size: renderref.SceneSize, Background: renderref.Background(), Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if h.RenderGPU() == nil {
			t.Skip("no GPU adapter available")
		}
	}

	const frames = 30
	measure := func(label string, mutate bool) {
		before := wgpu.DeviceStats()
		t0 := time.Now()
		for i := range frames {
			if mutate {
				animTick.Store(int32(i%20) + 1)
				h.core.Owner.RebuildAll()
			}
			h.RenderGPU()
		}
		d := time.Since(t0)
		made := wgpu.DeviceStats().Sub(before)
		enc := wgpu.EncoderStats()
		x := wgpu.TransferStats()
		t.Logf("%-22s %6.2f ms/frame  passes=%d draws=%d  made/frame %.1f buf %.1f bg  "+
			"bytes/frame: up %s (buf %s + tex %s) back %s",
			label, float64(d.Microseconds())/float64(frames)/1000,
			enc.RenderPasses, enc.DrawCalls,
			float64(made.Buffers)/frames, float64(made.BindGroups)/frames,
			human(x.Uploaded()), human(x.BufferBytes), human(x.TextureBytes),
			human(x.ReadbackBytes))
	}
	measure("full scene", false)
	measure("full scene, mutating", true)

	// The upper bound on what damage-rect scissoring could save: the same scene
	// with everything outside a small region clipped away. Headless has no
	// present path, so this cannot measure damage directly — it measures the
	// fragment work a scissor would remove, which is the ceiling for the render
	// half of Phase E. The present half (uploaded bytes) is not visible here.
	hc, err := NewHeadless(&clippedScene{}, Config{
		Size: renderref.SceneSize, Background: renderref.Background(), Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		hc.RenderGPU()
	}
	t0 := time.Now()
	for range frames {
		hc.RenderGPU()
	}
	t.Logf("%-22s %6.2f ms/frame   (ceiling for a scissored redraw)",
		"clipped to 48x48", float64(time.Since(t0).Microseconds())/float64(frames)/1000)
}

// clippedScene draws the same content with a spinner-sized clip, standing in
// for a render pass scissored to the damage rect.
type clippedScene struct{}

func (clippedScene) Build(ctx widget.Ctx) widget.Widget {
	return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(renderref.Background())
		c.PushClip(geom.RectXYWH(10, 290, 48, 48))
		for i := range 60 {
			x := float32(10 + (i%10)*30)
			y := float32(10 + (i/10)*36)
			c.FillRRect(geom.RectXYWH(x, y, 26, 30), 6,
				paint.RGB(0.25+float32(i%7)*0.09, 0.45, 0.8-float32(i%5)*0.08))
		}
		c.TextIn("", "42", geom.Pt{X: 14, Y: 300}, 18, paint.RGB(0, 0, 0))
		c.PopClip()
	}}
}

// animScene is a heavy background with one small changing label — the shape of
// a caret blink, a spinner, or a clock.
type animScene struct{}

func (animScene) Build(ctx widget.Ctx) widget.Widget {
	n := animTick.Load()
	return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.Clear(renderref.Background())
		// Heavy, unchanging background.
		for i := range 60 {
			x := float32(10 + (i%10)*30)
			y := float32(10 + (i/10)*36)
			c.FillRRect(geom.RectXYWH(x, y, 26, 30), 6,
				paint.RGB(0.25+float32(i%7)*0.09, 0.45, 0.8-float32(i%5)*0.08))
		}
		// The one small thing that changes.
		c.TextIn("", fmt.Sprintf("%02d", n), geom.Pt{X: 14, Y: 300}, 18, paint.RGB(0, 0, 0))
	}}
}

var animTick atomic.Int32

// human renders a byte count the way a person reads one.
func human(n uint64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
