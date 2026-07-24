//go:build !js

// Package desktop implements shell on macOS, Linux, and Windows by wrapping
// gogpu/gogpu (pure-Go windowing + WebGPU surface; zero CGo).
package desktop

import (
	"github.com/gogpu/gogpu"

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

	return app.Run()
}

type window struct {
	app *gogpu.App
}

func (w *window) Invalidate()          { w.app.RequestRedraw() }
func (w *window) SetTitle(title string) { w.app.SetTitle(title) }
func (w *window) Close()               { w.app.Quit() }

func (w *window) ClipboardRead() (string, error)  { return w.app.ClipboardRead() }
func (w *window) ClipboardWrite(text string) error { return w.app.ClipboardWrite(text) }

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

func (f *frame) Clear(r, g, b, a float32) { f.dc.Clear(r, g, b, a) }
