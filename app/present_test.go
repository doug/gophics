package app

// Tests for the shell-facing present path (app/present.go): the GPU replay
// skip for unchanged frames, target-transition re-renders (the swapchain
// question), and layout/paint panic isolation. These drive shellHandler.Frame
// directly with a fake window/frame/target — the Headless harness stops short
// of present(), which is exactly the seam under test.

import (
	"bytes"
	"image"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// fakeWindow is a minimal shell.Window for driving shellHandler.Frame.
type fakeWindow struct {
	dark        bool
	invalidated int
}

func (w *fakeWindow) Invalidate()                    { w.invalidated++ }
func (w *fakeWindow) SetTitle(string)                {}
func (w *fakeWindow) Close()                         {}
func (w *fakeWindow) ClipboardRead() (string, error) { return "", nil }
func (w *fakeWindow) ClipboardWrite(string) error    { return nil }
func (w *fakeWindow) OpenURL(string) error           { return nil }
func (w *fakeWindow) DarkMode() bool                 { return w.dark }

// fakeFrame is a shell.Frame whose target the test controls per frame.
type fakeFrame struct {
	size  geom.Size
	scale float32
	tgt   shell.Target
}

func (f *fakeFrame) Size() geom.Size      { return f.size }
func (f *fakeFrame) Scale() float32       { return f.scale }
func (f *fakeFrame) Target() shell.Target { return f.tgt }

// fakeGPUTarget implements gpuCanvasTarget + gpuSkipTarget, counting replays
// and deliberate skips. The replay runs against a real gg context so the
// recorded scene is actually exercised.
type fakeGPUTarget struct {
	renders int
	skips   int
	cc      *gg.Context
}

func (t *fakeGPUTarget) RenderGPU(replay func(*gg.Context)) {
	t.renders++
	replay(t.cc)
}
func (t *fakeGPUTarget) SkipRenderGPU() { t.skips++ }

func newFakeGPUTarget() *fakeGPUTarget {
	return &fakeGPUTarget{cc: gg.NewContextWithScale(200, 150, 1)}
}

// pendingTarget mimics a shell whose GPU device is still initializing: neither
// a GPU target nor a PixelTarget (the web shell's pre-ready sentinel).
type pendingTarget struct{}

// colorCanvas returns a root whose fill color is read at record time, so
// mutating *col changes the scene without any rebuild plumbing.
func colorCanvas(col *paint.Color) widget.Widget {
	return widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		c.FillRect(geom.RectFromSize(sz), *col)
	}}
}

func newPresentHarness(t *testing.T, root widget.Widget) (*shellHandler, *fakeWindow) {
	t.Helper()
	h, err := NewHandler(root, Config{Size: geom.Size{W: 200, H: 150}})
	if err != nil {
		t.Fatal(err)
	}
	return h.(*shellHandler), &fakeWindow{}
}

func TestUnchangedFrameSkipsGPUReplay(t *testing.T) {
	col := paint.RGB(0.2, 0.3, 0.4)
	sh, w := newPresentHarness(t, colorCanvas(&col))
	gt := newFakeGPUTarget()
	f := &fakeFrame{size: geom.Size{W: 200, H: 150}, scale: 1, tgt: gt}

	sh.Frame(w, f, 1.0/60) // first frame must render
	if gt.renders != 1 {
		t.Fatalf("first frame: renders = %d, want 1", gt.renders)
	}
	sh.Frame(w, f, 1.0/60) // nothing changed: replay skipped, skip signaled
	if gt.renders != 1 {
		t.Fatalf("unchanged frame must skip GPU replay: renders = %d, want 1", gt.renders)
	}
	if gt.skips != 1 {
		t.Fatalf("unchanged frame must signal the deliberate skip: skips = %d, want 1", gt.skips)
	}
	if !sh.core.Skipped {
		t.Fatal("unchanged GPU frame should report Skipped")
	}

	col = paint.RGB(0.9, 0.1, 0.1) // scene content changed
	sh.Frame(w, f, 1.0/60)
	if gt.renders != 2 {
		t.Fatalf("changed frame must replay: renders = %d, want 2", gt.renders)
	}
	if sh.core.Skipped {
		t.Fatal("changed GPU frame must not report Skipped")
	}

	sh.Frame(w, f, 1.0/60) // settled again
	if gt.renders != 2 || gt.skips != 2 {
		t.Fatalf("settled frame: renders = %d skips = %d, want 2 and 2", gt.renders, gt.skips)
	}
}

func TestResizeForcesGPUReplay(t *testing.T) {
	col := paint.RGB(0.2, 0.3, 0.4)
	sh, w := newPresentHarness(t, colorCanvas(&col))
	gt := newFakeGPUTarget()
	f := &fakeFrame{size: geom.Size{W: 200, H: 150}, scale: 1, tgt: gt}

	sh.Frame(w, f, 1.0/60)
	sh.Frame(w, f, 1.0/60)
	if gt.renders != 1 {
		t.Fatalf("setup: renders = %d, want 1", gt.renders)
	}
	f.size = geom.Size{W: 240, H: 150} // resize must re-render …
	sh.Frame(w, f, 1.0/60)
	if gt.renders != 2 {
		t.Fatalf("resized frame must replay: renders = %d, want 2", gt.renders)
	}
	f.scale = 2 // … and so must a scale change
	sh.Frame(w, f, 1.0/60)
	if gt.renders != 3 {
		t.Fatalf("rescaled frame must replay: renders = %d, want 3", gt.renders)
	}
}

// A GPU target the handler has never rendered to has never presented the
// scene — its swapchain may be brand-new (web pending→ready switchover, a
// recreated device surface after backgrounding). Even an unchanged scene must
// render there once.
func TestNewGPUTargetRendersUnchangedScene(t *testing.T) {
	col := paint.RGB(0.2, 0.3, 0.4)
	sh, w := newPresentHarness(t, colorCanvas(&col))
	f := &fakeFrame{size: geom.Size{W: 200, H: 150}, scale: 1, tgt: pendingTarget{}}

	sh.Frame(w, f, 1.0/60) // GPU still initializing: nothing presented
	gt := newFakeGPUTarget()
	f.tgt = gt
	sh.Frame(w, f, 1.0/60) // scene unchanged, but this surface never saw it
	if gt.renders != 1 {
		t.Fatalf("first frame on a new GPU target must render: renders = %d", gt.renders)
	}

	gt2 := newFakeGPUTarget() // device/surface recreation: new target identity
	f.tgt = gt2
	sh.Frame(w, f, 1.0/60)
	if gt2.renders != 1 {
		t.Fatalf("recreated GPU target must render even unchanged: renders = %d", gt2.renders)
	}
	if gt.renders != 1 {
		t.Fatalf("old target must not be touched: renders = %d", gt.renders)
	}
	sh.Frame(w, f, 1.0/60) // same target again, still unchanged: now skippable
	if gt2.renders != 1 || gt2.skips != 1 {
		t.Fatalf("settled frame on same target: renders = %d skips = %d, want 1 and 1", gt2.renders, gt2.skips)
	}
}

func TestPaintPanicDropsFrameKeepsAppAlive(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	col := paint.RGB(0.2, 0.3, 0.4)
	panicNow := false
	root := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		if panicNow {
			panic("paint boom")
		}
		c.FillRect(geom.RectFromSize(sz), col)
	}}
	sh, w := newPresentHarness(t, root)
	puts := 0
	f := &fakeFrame{size: geom.Size{W: 200, H: 150}, scale: 1,
		tgt: shell.PixelTarget{Put: func(*image.RGBA) { puts++ }}}

	sh.Frame(w, f, 1.0/60) // healthy frame renders and presents
	if puts != 1 {
		t.Fatalf("setup: puts = %d, want 1", puts)
	}

	panicNow = true
	for i := 0; i < 3; i++ { // a panicking frame must not crash the loop …
		sh.Frame(w, f, 1.0/60)
	}
	if sh.core.framePanics != 3 {
		t.Fatalf("framePanics = %d, want 3", sh.core.framePanics)
	}
	if got := strings.Count(buf.String(), "panic in layout/paint"); got != 1 {
		t.Fatalf("rapid repeat panics must be rate-limited to one log, got %d\n%s", got, buf.String())
	}
	if puts != 4 {
		// … and each dropped frame re-presents the retained previous surface.
		t.Fatalf("dropped frames should keep presenting the old surface: puts = %d, want 4", puts)
	}

	panicNow = false
	sh.Frame(w, f, 1.0/60) // the loop is still alive and renders again
	if puts != 5 {
		t.Fatalf("post-recovery frame should present: puts = %d, want 5", puts)
	}
	col = paint.RGB(0.9, 0.1, 0.1)
	sh.Frame(w, f, 1.0/60) // and still tracks changes normally
	if sh.core.Skipped {
		t.Fatal("changed frame after recovery must rasterize")
	}
}

func TestPaintPanicOnGPUTargetSkipsReplay(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	col := paint.RGB(0.2, 0.3, 0.4)
	panicNow := false
	root := widget.Canvas{Draw: func(c paint.Canvas, sz geom.Size) {
		if panicNow {
			panic("paint boom")
		}
		c.FillRect(geom.RectFromSize(sz), col)
	}}
	sh, w := newPresentHarness(t, root)
	gt := newFakeGPUTarget()
	f := &fakeFrame{size: geom.Size{W: 200, H: 150}, scale: 1, tgt: gt}

	sh.Frame(w, f, 1.0/60)
	panicNow = true
	sh.Frame(w, f, 1.0/60) // dropped: no replay, deliberate skip signaled
	if gt.renders != 1 {
		t.Fatalf("dropped frame must not replay: renders = %d, want 1", gt.renders)
	}
	if gt.skips != 1 {
		t.Fatalf("dropped frame must signal skip: skips = %d, want 1", gt.skips)
	}
	panicNow = false
	col = paint.RGB(0.9, 0.1, 0.1)
	sh.Frame(w, f, 1.0/60)
	if gt.renders != 2 {
		t.Fatalf("recovered frame must replay: renders = %d, want 2", gt.renders)
	}
}
