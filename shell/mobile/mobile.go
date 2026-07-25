// Package mobile implements shell for embedded hosts (Android/iOS): the
// host platform owns the surface and event loop and drives a Bridge —
// pushing input in, pulling RGBA frames out. This is the M9 embedding
// model (PLAN.md §6.4): Go as a library inside a thin native shell.
//
// The Bridge is pure Go and platform-agnostic, so the entire mobile
// contract is testable headless; the platform-specific part is only the
// host project forwarding calls (see examples/hn/android).
//
// Presentation is CPU pixels (like shell/web): the host blits the returned
// frame into its surface. A GPU surface path can replace the blit later
// without changing anything above shell.
package mobile

import (
	"image"
	"sync/atomic"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// Touch phases, mirroring Android's MotionEvent actions.
const (
	TouchDown = iota
	TouchMove
	TouchUp
	TouchCancel
)

// Bridge connects a host surface to a shell.Handler. All methods except
// NeedsFrame must be called from the host's UI thread.
type Bridge struct {
	handler shell.Handler

	widthPx, heightPx int
	scale             float32

	frame  *image.RGBA
	dirty  atomic.Bool
	dark   bool
	opened []string // OpenURL requests for the host to perform
	clip   string
}

// NewBridge wraps a shell.Handler (see app.NewHandler).
func NewBridge(h shell.Handler) *Bridge {
	b := &Bridge{scale: 1}
	b.handler = h
	b.dirty.Store(true)
	return b
}

// Resize sets the surface size in physical pixels and the density scale
// (logical px = physical / scale). Sends Resize to the handler.
func (b *Bridge) Resize(widthPx, heightPx int, scale float32) {
	if scale <= 0 {
		scale = 1
	}
	b.widthPx, b.heightPx, b.scale = widthPx, heightPx, scale
	b.handler.Event(b, shell.Resize{Size: b.logicalSize(), Scale: scale})
	b.dirty.Store(true)
}

func (b *Bridge) logicalSize() geom.Size {
	return geom.Size{W: float32(b.widthPx) / b.scale, H: float32(b.heightPx) / b.scale}
}

// NeedsFrame reports whether the UI requested a repaint (safe from any
// thread; the host checks each vsync).
func (b *Bridge) NeedsFrame() bool { return b.dirty.Load() }

// RenderFrame runs one frame and returns the surface as RGBA8888 pixels
// (widthPx*heightPx*4, row-major), or nil if the surface has no size.
// dtSeconds is the time since the previous frame.
func (b *Bridge) RenderFrame(dtSeconds float64) []byte {
	if b.widthPx == 0 || b.heightPx == 0 {
		return nil
	}
	b.dirty.Store(false)
	b.handler.Frame(b, &frame{b: b}, dtSeconds)
	if b.frame == nil {
		return nil
	}
	return b.frame.Pix
}

// Touch delivers a single-pointer touch event in physical pixels.
// (Multi-touch gestures — pinch — arrive with the gesture milestone.)
func (b *Bridge) Touch(phase int, xPx, yPx float32) {
	p := geom.Pt{X: xPx / b.scale, Y: yPx / b.scale}
	switch phase {
	case TouchDown:
		// Synthesize a move first so hover/hit state sees the position.
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerMove, Pos: p})
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerDown, Pos: p})
	case TouchMove:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerMove, Pos: p})
	case TouchUp:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerUp, Pos: p})
	case TouchCancel:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: -1e6, Y: -1e6}})
	}
}

// Text delivers committed text input from the on-screen keyboard.
func (b *Bridge) Text(s string) { b.handler.Event(b, shell.Text{S: s}) }

// Key delivers a key press by shell.KeyCode value (host maps its codes).
func (b *Bridge) Key(code int, pressed bool) {
	kind := shell.KeyRelease
	if pressed {
		kind = shell.KeyPress
	}
	b.handler.Event(b, shell.Key{Kind: kind, Code: shell.KeyCode(code)})
}

// Composition delivers IME preedit state (kind: 0 start, 1 update, 2 end).
func (b *Bridge) Composition(kind int, preedit string, cursor int, committed string) {
	b.handler.Event(b, shell.Composition{
		Kind:      shell.CompositionKind(kind),
		Preedit:   preedit,
		Cursor:    cursor,
		Committed: committed,
	})
}

// Focused reports app focus/visibility changes.
func (b *Bridge) Focused(focused bool) {
	b.handler.Event(b, shell.Focus{Focused: focused})
}

// TakeOpenedURL returns and clears the oldest pending OpenURL request
// ("" when none): the host opens it with an Intent/UIApplication.
func (b *Bridge) TakeOpenedURL() string {
	if len(b.opened) == 0 {
		return ""
	}
	u := b.opened[0]
	b.opened = b.opened[1:]
	return u
}

// SetClipboard pushes the host clipboard content into the bridge (the host
// syncs it); ClipboardText returns what the UI last wrote.
func (b *Bridge) SetClipboard(s string) { b.clip = s }
func (b *Bridge) ClipboardText() string { return b.clip }

// shell.Window implementation.

func (b *Bridge) Invalidate()          { b.dirty.Store(true) }
func (b *Bridge) SetTitle(string)      {}
func (b *Bridge) Close()               {}
func (b *Bridge) DarkMode() bool       { return b.dark }
func (b *Bridge) OpenURL(u string) error {
	b.opened = append(b.opened, u)
	return nil
}
func (b *Bridge) ClipboardRead() (string, error) { return b.clip, nil }
func (b *Bridge) ClipboardWrite(s string) error  { b.clip = s; return nil }

// SetDarkMode informs the UI of the host color scheme.
func (b *Bridge) SetDarkMode(dark bool) {
	b.dark = dark
	b.dirty.Store(true)
}

// shell.Frame implementation.

type frame struct{ b *Bridge }

func (f *frame) Size() geom.Size { return f.b.logicalSize() }
func (f *frame) Scale() float32  { return f.b.scale }
func (f *frame) Target() shell.Target {
	return shell.PixelTarget{Put: func(img *image.RGBA) { f.b.frame = img }}
}
