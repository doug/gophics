//go:build !js

// Package desktop implements shell on macOS, Linux, and Windows by wrapping
// gogpu/gogpu (pure-Go windowing + WebGPU surface; zero CGo).
package desktop

import (
	"fmt"
	"image"
	"log"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// Run opens a window and drives h until the window closes.
// It must be called from the main goroutine and blocks for the app lifetime.
func Run(h shell.Handler, cfg shell.Config) error {
	gcfg := gogpu.DefaultConfig()
	if cfg.Title != "" {
		gcfg.Title = cfg.Title
	}
	if !cfg.Size.IsEmpty() {
		gcfg.Width = int(cfg.Size.W)
		gcfg.Height = int(cfg.Size.H)
	}
	gcfg.Resizable = cfg.Resizable
	gcfg.VSync = true

	app := gogpu.NewApp(gcfg)
	w := &window{app: app}

	var dt float64
	app.OnUpdate(func(d float64) { dt = d })
	app.OnDraw(func(dc *gogpu.Context) {
		h.Frame(w, &frame{dc: dc}, dt)
	})
	app.OnResize(func(width, height int) {
		h.Event(w, shell.Resize{
			Size:  geom.Size{W: float32(width), H: float32(height)},
			Scale: w.scale(),
		})
	})
	app.OnFocus(func(focused bool) {
		h.Event(w, shell.Focus{Focused: focused})
	})
	app.OnClose(func() {
		h.Event(w, shell.Closed{})
	})

	es := app.EventSource()
	es.OnMouseMove(func(x, y float64) {
		h.Event(w, shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: float32(x), Y: float32(y)}})
	})
	es.OnMousePress(func(b gpucontext.MouseButton, x, y float64) {
		h.Event(w, shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: float32(x), Y: float32(y)}, Button: button(b)})
	})
	es.OnMouseRelease(func(b gpucontext.MouseButton, x, y float64) {
		h.Event(w, shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: float32(x), Y: float32(y)}, Button: button(b)})
	})
	es.OnScroll(func(dx, dy float64) {
		// gpucontext reports positive delta = scroll content down/right; the
		// gossamer convention (matched by the web shell) is the negated
		// platform delta, so forwarding it raw scrolled the wrong way. Negate
		// to align, and to honor the OS natural-scroll setting the platform
		// has already applied to these deltas.
		h.Event(w, shell.Pointer{Kind: shell.PointerScroll, Scroll: geom.Pt{X: -float32(dx), Y: -float32(dy)}})
	})
	es.OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		h.Event(w, shell.Key{Kind: shell.KeyPress, Code: keyCode(key), Mods: modBits(mods)})
	})
	es.OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		h.Event(w, shell.Key{Kind: shell.KeyRelease, Code: keyCode(key), Mods: modBits(mods)})
	})
	es.OnTextInput(func(text string) {
		h.Event(w, shell.Text{S: text})
	})
	es.OnIMECompositionStart(func() {
		h.Event(w, shell.Composition{Kind: shell.CompositionStart})
	})
	es.OnIMECompositionUpdate(func(state gpucontext.IMEState) {
		h.Event(w, shell.Composition{
			Kind:    shell.CompositionUpdate,
			Preedit: state.CompositionText,
			Cursor:  state.CursorPos,
		})
	})
	es.OnIMECompositionEnd(func(committed string) {
		h.Event(w, shell.Composition{Kind: shell.CompositionEnd, Committed: committed})
	})

	return app.Run()
}

func button(b gpucontext.MouseButton) uint8 {
	switch b {
	case gpucontext.MouseButtonLeft:
		return 0
	case gpucontext.MouseButtonRight:
		return 1
	case gpucontext.MouseButtonMiddle:
		return 2
	}
	return 0
}

func keyCode(k gpucontext.Key) shell.KeyCode {
	switch k {
	case gpucontext.KeyEnter:
		return shell.KeyEnter
	case gpucontext.KeyBackspace:
		return shell.KeyBackspace
	case gpucontext.KeyDelete:
		return shell.KeyDelete
	case gpucontext.KeyEscape:
		return shell.KeyEscape
	case gpucontext.KeyTab:
		return shell.KeyTab
	case gpucontext.KeyLeft:
		return shell.KeyLeft
	case gpucontext.KeyRight:
		return shell.KeyRight
	case gpucontext.KeyUp:
		return shell.KeyUp
	case gpucontext.KeyDown:
		return shell.KeyDown
	case gpucontext.KeyHome:
		return shell.KeyHome
	case gpucontext.KeyEnd:
		return shell.KeyEnd
	case gpucontext.KeyA:
		return shell.KeyA
	case gpucontext.KeyC:
		return shell.KeyC
	case gpucontext.KeyV:
		return shell.KeyV
	case gpucontext.KeyX:
		return shell.KeyX
	}
	return shell.KeyUnknown
}

func modBits(m gpucontext.Modifiers) shell.Mods {
	var out shell.Mods
	if m.HasShift() {
		out |= shell.ModShift
	}
	if m.HasControl() {
		out |= shell.ModCtrl
	}
	if m.HasAlt() {
		out |= shell.ModAlt
	}
	if m.HasSuper() {
		out |= shell.ModSuper
	}
	return out
}

type window struct {
	app *gogpu.App
}

func (w *window) Invalidate()           { w.app.RequestRedraw() }
func (w *window) SetTitle(title string) { w.app.SetTitle(title) }
func (w *window) Close()                { w.app.Quit() }

func (w *window) ClipboardRead() (string, error)   { return w.app.ClipboardRead() }
func (w *window) ClipboardWrite(text string) error { return w.app.ClipboardWrite(text) }

func (w *window) DarkMode() bool { return w.app.DarkMode() }

func (w *window) OpenURL(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("desktop: refusing to open non-http URL %q", url)
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (w *window) scale() float32 {
	lw, _ := w.app.Size()
	pw, _ := w.app.PhysicalSize()
	if lw <= 0 {
		return 1
	}
	return float32(pw) / float32(lw)
}

type frame struct {
	dc *gogpu.Context
}

func (f *frame) Size() geom.Size {
	return geom.Size{W: float32(f.dc.Width()), H: float32(f.dc.Height())}
}

func (f *frame) Scale() float32 {
	pw, _ := f.dc.FramebufferSize()
	if f.dc.Width() <= 0 {
		return 1
	}
	return float32(pw) / float32(f.dc.Width())
}

// Target presents CPU-rasterized frames by uploading them as a GPU texture
// and drawing it fullscreen (gogpu's universal PresentTexture path). This
// is the M1 model — CPU rasterizer, blit to the surface — shared with the
// web and mobile shells; it works on every backend (Metal/Vulkan/DX12 and
// the software adapter). A real GPU vector backend (PLAN.md M5) would
// present directly instead.
func (f *frame) Target() shell.Target {
	return shell.PixelTarget{Put: func(img *image.RGBA) {
		r := f.dc.Renderer()
		tex, err := r.NewTextureFromImage(img)
		if err != nil {
			log.Printf("gossamer/desktop: upload frame: %v", err)
			return
		}
		// PresentTexture submits an async GPU draw that samples tex; the GPU
		// may still be reading it after this returns. Destroying it now (the
		// old `defer tex.Destroy()`) freed it mid-flight, so a moving frame
		// sampled garbage — visible as trailing streaks, worst under slow
		// motion (many frames). Defer to the next frame's BeginFrame, which
		// runs after the GPU has consumed this frame.
		if err := f.dc.PresentTexture(tex); err != nil {
			log.Printf("gossamer/desktop: present: %v", err)
		}
		r.EnqueueDeferredDestroy(tex.Destroy)
	}}
}
