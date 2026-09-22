package mobile

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A GPU surface that configures but cannot present must give up.
//
// This is the worst failure this path has, and an Android emulator without
// working Vulkan produces it every time: SetSurface succeeds, GPUActive says
// yes, and every frame then fails to acquire a swapchain image. The host, told
// the GPU is live, renders into nothing — a black screen whose only explanation
// is sixty identical log lines a second.
//
// The contract is that GPUActive stops saying yes, so a host polling it each
// frame falls back to the CPU without needing to understand Vulkan.
func TestGPUSurfaceRetiresAfterRepeatedPresentFailures(t *testing.T) {
	b := NewBridge(nil)
	g := &mobileGPU{}
	b.gpu = g

	if !b.GPUActive() {
		t.Fatal("a fresh surface must report active")
	}

	// Transient failures do not retire it: a swapchain can break during a
	// resize and recover on the next frame.
	for i := 1; i < gpuFailureLimit; i++ {
		g.presentFailed("get current texture", errFake{})
		if !b.GPUActive() {
			t.Fatalf("retired after %d failure(s); the limit is %d, so a "+
				"transient break would cost the GPU for the whole session",
				i, gpuFailureLimit)
		}
	}

	g.presentFailed("get current texture", errFake{})
	if b.GPUActive() {
		t.Errorf("still active after %d consecutive failures — the host will keep "+
			"rendering into a surface that never presents", gpuFailureLimit)
	}

	// A successful present clears the count, so a surface that recovers is not
	// held against its earlier failures.
	g.failed = 0
	if !b.GPUActive() {
		t.Error("a recovered surface must report active again")
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake" }

// firstPaintApp is a blue page with a small centred square that turns red on
// tap — the smallest change that leaves the damage rect a fraction of the
// surface, standing in for the blinking caret the bug was found with.
type firstPaintApp struct{}

func (firstPaintApp) CreateState() widget.State { return &firstPaintState{} }

type firstPaintState struct {
	widget.StateBase[firstPaintApp]
	tapped bool
}

func (s *firstPaintState) Build(ctx widget.Ctx) widget.Widget {
	sq := paint.RGB(0.1, 0.9, 0.1)
	if s.tapped {
		sq = paint.RGB(0.9, 0.1, 0.1)
	}
	return widget.Decorated{Color: paint.RGB(0.1, 0.1, 0.9), Child: widget.Center(
		widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { s.SetState(func() { s.tapped = true }) }},
			Child:    widget.Decorated{Color: sq, Child: widget.Sized{W: 40, H: 40}},
		},
	)}
}

// The first CPU frame after the GPU retires must be a full paint.
//
// On an Android emulator without working Vulkan the GPU surface configures,
// every RenderFrame fails to present, and after gpuFailureLimit frames the
// host switches to Snapshot + blit. Those GPU frames still recorded scenes and
// advanced the diff baseline, but nothing ever reached the painter's CPU
// surface — so the first Snapshot diffed against a frame the CPU pixmap never
// held, painted only what had changed since (a text caret) onto a fresh zeroed
// buffer, and the user saw a caret blinking on black until a tap dirtied the
// whole screen. The switch itself must invalidate everything.
func TestFirstCPUFrameAfterGPURetirementIsFullyPainted(t *testing.T) {
	h, err := app.NewHandler(firstPaintApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := NewBridge(h)
	b.Resize(200, 300, 1)

	// A surface that configured but cannot present, driven the way the host
	// drives it: RenderFrame each vsync while NeedsFrame and GPUActive say
	// yes. The scene is static, which is the harder case: an unchanged scene
	// normally skips the GPU replay, and a failed present must not count as
	// having shown it or the surface never retries and never retires.
	b.gpu = &mobileGPU{pw: 200, ph: 300, scale: 1}
	for i := 0; i < gpuFailureLimit; i++ {
		if !b.GPUActive() {
			t.Fatalf("setup: GPU retired after %d frame(s), before the limit of %d", i, gpuFailureLimit)
		}
		if !b.NeedsFrame() {
			t.Fatalf("frame %d: a present that failed must leave a frame wanted, or the host stops calling", i)
		}
		b.RenderFrame(1.0 / 60)
	}
	if b.GPUActive() {
		t.Fatal("setup: the GPU should have retired after the failure limit")
	}

	// Something small changes between the last GPU frame and the first CPU
	// one, so a diff-derived damage rect would cover only the square.
	b.Touch(TouchDown, 100, 150)
	b.Touch(TouchUp, 100, 150)

	// What MainActivity does once gpuActive() turns false: release the surface
	// and present the CPU snapshot.
	b.ClearSurface()
	pix := b.Snapshot(1.0 / 60)
	if pix == nil {
		t.Fatal("first CPU frame after GPU retirement produced no snapshot")
	}
	w, hgt := b.FrameWidth(), b.FrameHeight()
	if len(pix) != w*hgt*4 {
		t.Fatalf("snapshot bytes = %d, want %d", len(pix), w*hgt*4)
	}
	unpainted := 0
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 255 {
			unpainted++
		}
	}
	if unpainted > 0 {
		t.Fatalf("%d of %d pixels unpainted on the first CPU frame: the damage "+
			"rect carried over from GPU frames the CPU surface never held",
			unpainted, w*hgt)
	}
	if r, bl := pix[0], pix[2]; r > 100 || bl < 100 {
		t.Fatalf("corner should be the blue background, got r=%d b=%d", r, bl)
	}
	if ci := ((hgt/2)*w + w/2) * 4; pix[ci] < 100 || pix[ci+2] > 100 {
		t.Fatalf("centre should be the tapped (red) square, got r=%d b=%d", pix[ci], pix[ci+2])
	}
}
