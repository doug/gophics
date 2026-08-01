// Package mobile implements shell for embedded hosts (Android/iOS): the
// host platform owns the surface and event loop and drives a Bridge —
// pushing input in, presenting frames out. This is the M9 embedding
// model (PLAN.md §6.4): Go as a library inside a thin native shell.
//
// Rendering is GPU-first: the host hands over a native render surface
// (SetSurface — iOS CAMetalLayer / Android ANativeWindow) and RenderFrame
// rasterizes straight to it via gg's accelerator (see gpu.go), matching the
// web/desktop GPU present. Snapshot renders a frame offscreen on the CPU for
// headless tests and screenshots, so the Bridge contract stays fully testable
// without a device. See examples/hn/{ios,android} for host wiring.
package mobile

import (
	"image"
	"sync/atomic"

	"github.com/doug/gossamer/app"
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

	frame        *image.RGBA
	dirty        atomic.Bool
	dark         bool
	a11y         []app.A11yNode
	opened       []string // OpenURL requests for the host to perform
	clip         string
	media        *mediaBridge // camera/audio capability plumbing (see media.go)
	gpu          *mobileGPU   // GPU surface the host renders to (see gpu.go)
	snapshotting bool         // force a CPU offscreen frame (Snapshot)
}

// NewBridge wraps a shell.Handler (see app.NewHandler).
func NewBridge(h shell.Handler) *Bridge {
	b := &Bridge{scale: 1}
	b.handler = h
	b.media = newMediaBridge(b)
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
	if b.gpu != nil {
		b.gpu.resize(widthPx, heightPx, float64(scale))
	}
	b.handler.Event(b, shell.Resize{Size: b.logicalSize(), Scale: scale})
	b.dirty.Store(true)
}

func (b *Bridge) logicalSize() geom.Size {
	return geom.Size{W: float32(b.widthPx) / b.scale, H: float32(b.heightPx) / b.scale}
}

// NeedsFrame reports whether the UI requested a repaint (safe from any
// thread; the host checks each vsync).
func (b *Bridge) NeedsFrame() bool { return b.dirty.Load() }

// RenderFrame runs one frame and presents it on the GPU to the host surface
// (provided via SetSurface). Call it each vsync while NeedsFrame is true.
// Rendering is GPU-only; before a surface is set it is a no-op.
func (b *Bridge) RenderFrame(dtSeconds float64) {
	if b.gpu == nil || b.widthPx == 0 || b.heightPx == 0 {
		return
	}
	b.dirty.Store(false)
	b.handler.Frame(b, &frame{b: b}, dtSeconds)
}

// Snapshot renders one frame offscreen on the CPU and returns it as RGBA8888
// pixels (FrameWidth*FrameHeight*4, row-major) — for headless tests and
// screenshots, independent of the live GPU surface. nil if the surface has no
// size.
func (b *Bridge) Snapshot(dtSeconds float64) []byte {
	if b.widthPx == 0 || b.heightPx == 0 {
		return nil
	}
	b.snapshotting = true
	b.dirty.Store(false)
	b.handler.Frame(b, &frame{b: b}, dtSeconds)
	b.snapshotting = false
	if b.frame == nil {
		return nil
	}
	return b.frame.Pix
}

// FrameWidth and FrameHeight are the pixel dimensions of the last Snapshot
// (they can differ from the surface size by a rounding pixel).
func (b *Bridge) FrameWidth() int {
	if b.frame == nil {
		return 0
	}
	return b.frame.Rect.Dx()
}

func (b *Bridge) FrameHeight() int {
	if b.frame == nil {
		return 0
	}
	return b.frame.Rect.Dy()
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

// SetInsets delivers safe-area insets in physical pixels (status bar,
// gesture bars, keyboard).
func (b *Bridge) SetInsets(topPx, rightPx, bottomPx, leftPx float32) {
	s := b.scale
	b.handler.Event(b, shell.Insets{Insets: geom.Insets{
		Top: topPx / s, Right: rightPx / s, Bottom: bottomPx / s, Left: leftPx / s,
	}})
}

// TextInputActive reports whether the UI wants keyboard input; the host
// shows/hides the on-screen keyboard on transitions.
func (b *Bridge) TextInputActive() bool {
	if ta, ok := b.handler.(interface{ TextInputActive() bool }); ok {
		return ta.TextInputActive()
	}
	return false
}

// Accessibility: the host (Android AccessibilityNodeProvider, iOS
// UIAccessibility) refreshes the flat node tree, then reads nodes by index
// and activates by ID. Rects are physical pixels.

type a11yProvider interface {
	A11yTree(scale float32) []app.A11yNode
	A11yActivate(id int)
	A11yHitTest(x, y int, scale float32) int
}

// A11yRefresh rebuilds the accessibility node tree and returns the node
// count (0 if the handler provides no semantics).
func (b *Bridge) A11yRefresh() int {
	if p, ok := b.handler.(a11yProvider); ok {
		b.a11y = p.A11yTree(b.scale)
	} else {
		b.a11y = nil
	}
	return len(b.a11y)
}

func (b *Bridge) a11yAt(i int) *app.A11yNode {
	if i < 0 || i >= len(b.a11y) {
		return nil
	}
	return &b.a11y[i]
}

// Flat per-index accessors (gomobile-friendly).
func (b *Bridge) A11yID(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return -1
	}
	return n.ID
}
func (b *Bridge) A11yParent(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return -1
	}
	return n.ParentID
}
func (b *Bridge) A11yRole(i int) string {
	n := b.a11yAt(i)
	if n == nil {
		return ""
	}
	return n.Role
}
func (b *Bridge) A11yLabel(i int) string {
	n := b.a11yAt(i)
	if n == nil {
		return ""
	}
	return n.Label
}
func (b *Bridge) A11yValue(i int) string {
	n := b.a11yAt(i)
	if n == nil {
		return ""
	}
	return n.Value
}
func (b *Bridge) A11yHint(i int) string {
	n := b.a11yAt(i)
	if n == nil {
		return ""
	}
	return n.Hint
}
func (b *Bridge) A11yX(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return 0
	}
	return n.X
}
func (b *Bridge) A11yY(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return 0
	}
	return n.Y
}
func (b *Bridge) A11yW(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return 0
	}
	return n.W
}
func (b *Bridge) A11yH(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return 0
	}
	return n.H
}
func (b *Bridge) A11yTappable(i int) bool { n := b.a11yAt(i); return n != nil && n.Tappable }
func (b *Bridge) A11yChildCount(i int) int {
	n := b.a11yAt(i)
	if n == nil {
		return 0
	}
	return len(n.Children)
}
func (b *Bridge) A11yChild(i, j int) int {
	n := b.a11yAt(i)
	if n == nil || j < 0 || j >= len(n.Children) {
		return -1
	}
	return n.Children[j]
}

// A11yActivate fires the node's action (screen-reader activate).
func (b *Bridge) A11yActivate(id int) {
	if p, ok := b.handler.(a11yProvider); ok {
		p.A11yActivate(id)
		b.dirty.Store(true)
	}
}

// A11yHitTest returns the node ID at a physical-pixel point (explore by
// touch), or -1.
func (b *Bridge) A11yHitTest(xPx, yPx int) int {
	if p, ok := b.handler.(a11yProvider); ok {
		return p.A11yHitTest(xPx, yPx, b.scale)
	}
	return -1
}

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

func (b *Bridge) Invalidate()     { b.dirty.Store(true) }
func (b *Bridge) SetTitle(string) {}
func (b *Bridge) Close()          {}
func (b *Bridge) DarkMode() bool  { return b.dark }
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

// Target presents on the GPU surface for a live frame; a Snapshot forces a CPU
// offscreen target instead (tests/screenshots).
func (f *frame) Target() shell.Target {
	if !f.b.snapshotting && f.b.gpu != nil {
		return mobileGPUTarget{f.b.gpu}
	}
	return shell.PixelTarget{Put: func(img *image.RGBA) { f.b.frame = img }}
}
