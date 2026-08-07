// Package shell defines the platform interface gophics runs on: a window (or
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
	"image"
	"os"
	"strings"

	"github.com/doug/gophics/internal/gfx/gpucontext"

	"github.com/doug/gophics/geom"
)

// RendererMode selects the rasterization + presentation backend. The zero
// value (Auto) prefers the GPU and falls back to the CPU rasterizer when no
// GPU is available.
type RendererMode uint8

const (
	// RendererAuto prefers the GPU, falling back to CPU when unavailable.
	RendererAuto RendererMode = iota
	// RendererGPU forces the GPU path (still falls back to CPU if the platform
	// truly has no GPU, since a blank window is never the right answer).
	RendererGPU
	// RendererCPU forces the deterministic CPU rasterizer.
	RendererCPU
)

// ResolveRenderer applies the GOPHICS_RENDERER environment override (values
// "auto", "gpu", or "cpu"; case-insensitive) on top of the mode a program
// requested, so a build can be flipped to CPU/GPU without recompiling. An
// unset or unrecognized variable leaves the requested mode unchanged. (On the
// web os.Getenv is empty, so the requested mode wins there.)
func ResolveRenderer(requested RendererMode) RendererMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOPHICS_RENDERER"))) {
	case "cpu":
		return RendererCPU
	case "gpu":
		return RendererGPU
	case "auto":
		return RendererAuto
	default:
		return requested
	}
}

// Config describes the initial window configuration.
type Config struct {
	Title string
	// Size is the initial logical size. Zero means a platform default.
	Size      geom.Size
	Resizable bool
	// Renderer selects the rasterization backend (default Auto: GPU with CPU
	// fallback). Shells resolve GPU availability at runtime.
	Renderer RendererMode
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

	// OpenURL opens url in the system browser (or a new tab on web).
	OpenURL(url string) error

	// DarkMode reports whether the platform prefers a dark color scheme.
	DarkMode() bool
}

// Frame gives the handler access to one vsync-paced frame.
type Frame interface {
	// Size is the current logical size of the surface.
	Size() geom.Size
	// Scale is the device pixel ratio (physical / logical).
	Scale() float32
	// Target returns this frame's presentation target — one of the concrete
	// types below. The paint package type-switches on it.
	Target() Target
}

// Target is a frame's presentation target — deliberately an open type, not a
// sealed union: the present path (app.present) type-switches on it, so besides
// the two portable kinds here (GPUTarget, PixelTarget) a backend may return its
// own private target type that the present path recognizes by interface (e.g. a
// GPU-canvas target exposing RenderGPU). Consumers must handle an unknown target
// gracefully rather than assume the set is closed.
type Target any

// GPUTarget presents by compositing onto a WebGPU texture view.
type GPUTarget struct {
	View gpucontext.TextureView
	W, H int // physical pixels
}

// PixelTarget presents by handing finished physical-pixel frames to Put
// (e.g. a browser canvas or a test sink).
type PixelTarget struct {
	Put func(img *image.RGBA)
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
	// Source is the input device: mouse, touch, or pen. It lets gestures adapt
	// to modality (e.g. drag selects text with a mouse but scrolls on touch,
	// where selection is long-press-initiated). Zero value is a mouse.
	Source PointerSource
}

// PointerSource is the input device behind a Pointer event.
type PointerSource uint8

const (
	SourceMouse PointerSource = iota
	SourceTouch
	SourcePen
)

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
	KeyHome
	KeyEnd
	// Letter keys are delivered only for command shortcuts (a modifier
	// beyond Shift is held); plain typing arrives as Text.
	KeyA
	KeyC
	KeyV
	KeyX

	// Game/physical keys. APPEND ONLY, and never reorder the block above — the
	// mobile Bridge.Key ABI passes these integer values across the FFI boundary
	// (examples/hn/.../MainActivity.kt hardcodes indices). Delivered as physical
	// KeyPress/KeyRelease for held-state polling (input package), independent of
	// text input.
	KeySpace
	KeyW
	KeyS
	KeyD
	KeyE
	KeyQ
	KeyR
	KeyF
	KeyShift
	KeyCtrl
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
)

// Mods is a bitmask of held modifier keys.
type Mods uint8

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
	// ModSuper is Cmd on macOS, Win elsewhere.
	ModSuper
)

// Command reports whether the platform's primary command modifier is held
// (Cmd on macOS/web-mac, Ctrl elsewhere). Shells set the platform bit.
func (m Mods) Command() bool { return m&(ModCtrl|ModSuper) != 0 }

// Key is a physical key event. Text input arrives separately via Text
// (and, later, IME composition events — the shell interface reserves that
// path from the start; see PLAN.md §6.1).
type Key struct {
	Kind KeyKind
	Code KeyCode
	Mods Mods
}

// Text is committed text input (post-IME).
type Text struct{ S string }

// Insets reports platform-obstructed edges (status bar, notch, on-screen
// keyboard) in logical pixels.
type Insets struct{ Insets geom.Insets }

// CompositionKind discriminates Composition events.
type CompositionKind uint8

const (
	CompositionStart CompositionKind = iota
	CompositionUpdate
	CompositionEnd
)

// Composition reports IME preedit state (e.g. pinyin or kana being
// composed). During composition the preedit text is displayed inline with
// underline styling at the caret; CompositionEnd carries the committed
// text (which is NOT also delivered as Text).
type Composition struct {
	Kind    CompositionKind
	Preedit string // current composition text (empty on End)
	Cursor  int    // caret position within Preedit, in runes
	// Committed is the final text, set only for CompositionEnd.
	Committed string
}

func (Insets) isEvent()      {}
func (Resize) isEvent()      {}
func (Closed) isEvent()      {}
func (Focus) isEvent()       {}
func (Pointer) isEvent()     {}
func (Key) isEvent()         {}
func (Text) isEvent()        {}
func (Composition) isEvent() {}
