// Package shell defines the platform interface gophics runs on: a Window (or
// surface) that delivers input Events and vsync-paced Frames to a Handler,
// with each Frame exposing a presentation Target (GPU texture view or pixel
// sink) that the framework renders into.
//
// Platform implementations live in subpackages (shell/desktop, shell/web,
// shell/mobile, shell/terminal). The framework never imports those —
// applications pick one in main(). Everything above shell is platform-agnostic.
//
// The contract:
//   - Sizes and positions are in logical pixels; Frame.Scale converts to
//     physical device pixels.
//   - Rendering is on-demand: a frame is delivered only after Invalidate (or a
//     platform-initiated reason such as resize/expose). Continuous animation
//     re-requests inside Frame.
//   - All Handler calls happen on the UI goroutine.
//
// Beyond the core window/frame/event contract, optional platform integration
// (file pickers, share sheets, haptics, notifications, storage, ...) follows
// the capability pattern (see internal/capgen/README.md): each capability is
// a small interface in this package plus a <X>Window opt-in interface a
// platform Window implements when it can provide it; callers reach
// capabilities through the widget layer, which returns nil where the running
// platform has no support.
//
// Capability callbacks follow one nil rule, on every platform: a method whose
// only purpose is to deliver a result (Open, Capture, Authorize) is a no-op
// when its callback is nil, because doing the work with nobody to receive it
// is at best waste and at worst a leaked resource — a Record with no Recorder
// handle is a live microphone nothing can stop. A method that performs a side
// effect (Save, Share, Write, a recorder's Stop) proceeds regardless and
// skips only the report, because "fire and forget" is a legitimate way to
// call it. The rule lives here because it used to live nowhere: each backend
// answered for itself, and a nil callback was a no-op on one platform and a
// panic on another — the exact platform divergence this layer exists to
// prevent.
package shell

import (
	"image"
	"os"
	"strings"

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
	// AppID identifies the application for per-user storage locations (the
	// preferences file's directory). Use a stable, filesystem-safe name — a
	// reverse-DNS string like "com.example.tally" is conventional. Empty falls
	// back to the executable's name, which is stable for a shipped binary but
	// changes if the binary is renamed.
	AppID string
	// Size is the initial logical size. Zero means a platform default.
	Size      geom.Size
	Resizable bool
	// ScaleToFit treats Size as a fixed design size rather than an initial
	// one: the app always lays out at exactly Size, and the shell scales the
	// result to fit the display, letterboxed, preserving aspect ratio.
	//
	// Set it for a layout that genuinely does not reflow — a drum machine whose
	// pads are a fixed grid, a board game. Such an app on a phone held upright
	// is otherwise cropped or crushed, and shrinking it whole is the honest
	// answer. Leave it false for anything responsive: a list or a form should
	// use the space it is given, and scaling it down would only make it small.
	//
	// Currently honoured by the web shell.
	ScaleToFit bool
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
// the portable PixelTarget a backend may return its own private target type
// that the present path recognizes by interface (a GPU-canvas target exposing
// RenderGPU, as shell/mobile does). Consumers must handle an unknown target
// gracefully rather than assume the set is closed.
//
// GPU presentation is in-module only. There was a portable GPUTarget carrying a
// WebGPU texture view, and nothing ever constructed one: every GPU backend
// returns its own type satisfying the RenderGPU interface instead, so the
// branch was unreachable. It is not re-addable as a seam either — two Go WebGPU
// bindings cannot exchange a Device through Go types, so a foreign device
// cannot be handed in. An embedding host presents through PixelTarget.
type Target any

// PixelTarget presents by handing finished physical-pixel frames to Put
// (e.g. a browser canvas or a test sink).
//
// damage is the region of img that changed since the previous Put, in physical
// pixels. A host whose destination retains the last frame may upload only that
// rect; one that cannot must upload all of img and ignore it. It is the whole
// surface after a resize or the first frame, and empty when nothing changed —
// an empty rect means the host may skip the upload entirely.
//
// The runtime has always computed this: present() calls ReplayDamaged with the
// rect and redraws only the dirty region, then used to hand Put the whole
// surface, so every embedder re-uploaded every pixel of a frame that had
// changed one button. The damage was computed and thrown away at the last step.
type PixelTarget struct {
	Put func(img *image.RGBA, damage geom.Rect)
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
	Kind PointerKind
	// Pos is where the pointer is, and every shell sets it on every Pointer
	// event including PointerScroll — a scroll is delivered to whatever is
	// under the pointer, so a scroll without a position has nowhere to go. A
	// platform whose scroll callback reports only deltas is expected to carry
	// the last position it saw rather than leave this zero.
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

// KeyCode identifies a physical key, independent of layout. Only the codes
// the framework and its examples need are named; unlisted keys arrive as
// KeyUnknown. New codes are appended as needs arise (see the append-only
// note below).
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

// Key is a physical key event. Text input arrives separately: committed text
// via Text, and in-progress IME preedit via Composition.
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

// KeyboardInset reports the height of the on-screen keyboard, in logical pixels,
// measured from the bottom of the window. Zero means it is hidden.
//
// It is deliberately not folded into Insets. Safe-area insets are static per
// orientation and describe hardware; the keyboard is transient and describes a
// mode. A layout that should slide under the home indicator still must not be
// covered by the keyboard, and a screen with no text input should not reflow just
// because some other screen raised one — conflating them makes both wrong.
type KeyboardInset struct{ Height float32 }

// CapabilitiesChanged tells the runtime to re-read the window's capabilities.
//
// They are wired once when the shell hands over a Window, which is right for a
// set fixed at startup and wrong for one that is not: a mobile host registers
// its platform backends after Start, and connectivity and battery only become
// answerable once the platform has reported them once. Without this the
// capability is read as nil on the first frame and stays nil for the life of
// the window, however available it later becomes.
//
// A shell sends it when a capability appears or disappears. Re-wiring is cheap
// and idempotent, but it is not free — it rebuilds the Posted adapters — so it
// is an event rather than a per-frame check.
type CapabilitiesChanged struct{}

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

func (Insets) isEvent()              {}
func (KeyboardInset) isEvent()       {}
func (CapabilitiesChanged) isEvent() {}
func (Resize) isEvent()              {}
func (Closed) isEvent()              {}
func (Focus) isEvent()               {}
func (Pointer) isEvent()             {}
func (Key) isEvent()                 {}
func (Text) isEvent()                {}
func (Composition) isEvent()         {}
