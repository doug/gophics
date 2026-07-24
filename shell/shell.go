// Package shell defines the platform interface gossamer runs on: a window (or
// surface) that delivers input events and vsync-paced frames to a Handler.
//
// Platform implementations live in subpackages (shell/desktop, shell/web,
// later shell/android, shell/ios). The framework never imports those —
// applications pick one in main(). Everything above shell is platform-agnostic.
//
// Design notes (to be ratified as ADRs during M0):
//   - Sizes and positions are in logical pixels; Frame.Scale converts to
//     physical device pixels.
//   - Rendering is on-demand: a frame is delivered only after Invalidate (or a
//     platform-initiated reason such as resize/expose). Continuous animation
//     re-requests inside Frame.
//   - Frame.Clear is an M0 placeholder. It will be replaced by handing the
//     shell a composited scene (scene.Scene) once the paint/scene layers exist.
package shell

import (
	"github.com/gogpu/gpucontext"

	"github.com/doug/gossamer/geom"
)

// Config describes the initial window configuration.
type Config struct {
	Title string
	// Size is the initial logical size. Zero means a platform default.
	Size      geom.Size
	Resizable bool
}

// Window is the running platform window, usable from the UI goroutine.
type Window interface {
	// Invalidate requests that a new frame be delivered soon.
	Invalidate()
	SetTitle(title string)
	// Close requests window close; the handler receives Closed.
	Close()

	ClipboardRead() (string, error)
	ClipboardWrite(text string) error
}

// Frame gives the handler access to one vsync-paced frame.
type Frame interface {
	// Size is the current logical size of the surface.
	Size() geom.Size
	// Scale is the device pixel ratio (physical / logical).
	Scale() float32
	// Clear fills the frame with a color. M0 placeholder — see package docs.
	Clear(r, g, b, a float32)
	// View returns the frame's GPU render target and its physical pixel size.
	// M0-provisional: consumed by the paint package; replaced by a scene
	// handoff once paint/scene exist.
	View() (view gpucontext.TextureView, width, height int)
}

// Handler receives frames and events. All calls happen on the UI goroutine.
type Handler interface {
	// Frame is called when the platform is ready for the next frame.
	// dt is the time in seconds since the previous frame.
	Frame(w Window, f Frame, dt float64)
	// Event is called for every input, lifecycle, or window event.
	Event(w Window, e Event)
}

// Event is a platform event. The concrete types below are the complete set;
// handlers type-switch over them.
type Event interface{ isEvent() }

// Resize reports a new logical surface size and/or scale factor.
type Resize struct {
	Size  geom.Size
	Scale float32
}

// Closed reports that the window has been closed. It is the last event.
type Closed struct{}

// Focus reports keyboard focus gain/loss.
type Focus struct{ Focused bool }

// PointerKind discriminates Pointer events.
type PointerKind uint8

const (
	PointerMove PointerKind = iota
	PointerDown
	PointerUp
	PointerScroll
)

// Pointer is a mouse/touch/stylus event in logical coordinates.
type Pointer struct {
	Kind   PointerKind
	Pos    geom.Pt
	Button uint8   // 0=primary, 1=secondary, 2=middle (valid for Down/Up)
	Scroll geom.Pt // valid for PointerScroll, in logical pixels
}

// KeyKind discriminates Key events.
type KeyKind uint8

const (
	KeyPress KeyKind = iota
	KeyRelease
)

// KeyCode identifies a physical key, independent of layout. The full key
// model is a pending M0 ADR; until then only the codes the framework itself
// needs are named, and unlisted keys arrive as KeyUnknown.
type KeyCode uint32

const (
	KeyUnknown KeyCode = iota
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyEscape
	KeyTab
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
)

// Key is a physical key event. Text input arrives separately via Text
// (and, later, IME composition events — the shell interface reserves that
// path from the start; see PLAN.md §6.1).
type Key struct {
	Kind KeyKind
	Code KeyCode
}

// Text is committed text input (post-IME).
type Text struct{ S string }

func (Resize) isEvent()  {}
func (Closed) isEvent()  {}
func (Focus) isEvent()   {}
func (Pointer) isEvent() {}
func (Key) isEvent()     {}
func (Text) isEvent()    {}
