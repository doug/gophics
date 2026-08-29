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
	"sync"
	"sync/atomic"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
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
	announce     []announcement     // pending live-region speech; see a11y.go
	announceLast bool               // priority of the last TakeAnnouncement
	opened       []string           // OpenURL requests for the host to perform
	haptics      []shell.HapticKind // pending haptic events for the host to play
	prefs        *prefs.Store       // settings store; nil until SetFilesDir (prefs.go)

	// Platform state the host pushes rather than Go requesting; see
	// connectivity.go. Each "known" flag distinguishes "the host has not wired
	// this" from a real value, so the capability can be nil rather than lying.
	netKnown    bool
	online      bool
	netSubs     []func(bool)
	batKnown    bool
	batLevel    float32
	batCharging bool
	batSubs     []func()
	localeTag   string // BCP-47 from the host; see locale.go
	localeSub   func(string)
	initialLink string
	shareHost   ShareHost
	shareCb     map[int]func(error)
	notifyHost  NotifyHost
	notifyCb    map[int]func(shell.Permission)
	secureHost  SecureHost
	fileHost    FileHost
	pickCb      map[int]func([]shell.PickedFile, error)
	picked      map[int][]shell.PickedFile
	saveCb      map[int]func(error)
	locHost     LocationHost
	locOnce     map[int]func(lat, lon, acc float64, err error)
	locWatch    map[int]func(lat, lon, acc float64)
	deviceHost  DeviceHost
	wakeCount   int // outstanding screen-wake leases; see device.go
	photoPermCb map[int]func(shell.Permission)
	photoSaveCb map[int]func(error)
	authCb      map[int]func(bool, error)
	reqNext     int  // request ids for the capabilities in platform.go
	linkStarted bool // a frame has run, so a later link is not the "initial" one
	linkSubs    []func(string)

	clip         string           // cached system pasteboard; see SetClipboardText
	clipHas      bool             // host's answer to "is there text?"; see SetClipboardHasText
	clipHasSet   bool             // whether the host has ever answered, so "" stays distinguishable
	clipHost     ClipboardHost    // reads the pasteboard on demand; nil until a host registers
	clipPending  bool             // app copied, host has not drained it yet
	media        *mediaBridge     // shared one-shot request plumbing (see media.go)
	gpu          *mobileGPU       // GPU surface the host renders to (see gpu.go)
	dispHandle   int64            // retained native handles so Resize can full-rebuild
	winHandle    int64            // the GPU surface on an orientation change (see Resize)
	snapshotting bool             // force a CPU offscreen frame (Snapshot)
	capturing    gpuCaptureTarget // non-nil only while CaptureGPU is running

	// Lifecycle state, written from the host UI thread via SetAppState and
	// read by the widget tree; see lifecycle.go.
	lcMu    sync.Mutex
	lcState shell.AppState
	lcSubs  []func(shell.AppState)

	// On-screen keyboard / IME state, written by the TextInput capability on
	// the UI goroutine and read by the host through the accessors in
	// textinput.go. No lock: every one of those runs on the host UI thread.
	tiActive      bool
	tiType        shell.TextInputType
	tiAutocorrect bool
	tiText        string
	tiSelStart    int
	tiSelEnd      int
	tiRev         int
	tiReplace     func(start, end int, s string) // the focused field's OnReplace

	// Live microphone monitoring; see microphone.go. Guarded by its own mutex
	// because PCM blocks arrive on the audio thread, not the UI thread.
	monMu    sync.Mutex
	monHost  MonitorHost
	monitors map[int]*mobileMonitor
	monCb    map[int]func(shell.Monitor, error)

	// Live camera preview; see camera.go. Its own mutex for the same reason
	// as the microphone's: frames arrive on the camera thread.
	prevMu   sync.Mutex
	prevHost PreviewHost
	previews map[int]*mobilePreview
	prevCb   map[int]func(shell.Frames, error)
}

// NewBridge wraps a shell.Handler (see app.NewHandler).
func NewBridge(h shell.Handler) *Bridge {
	b := &Bridge{scale: 1}
	b.handler = h
	b.media = newMediaBridge(b)
	b.monitors = map[int]*mobileMonitor{}
	b.monCb = map[int]func(shell.Monitor, error){}
	b.previews = map[int]*mobilePreview{}
	b.prevCb = map[int]func(shell.Frames, error){}
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
		if b.gpu.orientationChanged(widthPx, heightPx) {
			// Orientation flip: fully rebuild the GPU surface rather than resize
			// it. Reconfiguring/resizing in place leaves the gg session's MSAA
			// textures mismatched with the new swapchain and the resolve shears
			// the whole frame in landscape on PowerVR (a fresh build into the
			// same orientation is clean; a resize into it tears). Rebuilding is
			// the proven-clean path. Rotation is rare, so the cost is fine.
			b.SetSurface(b.dispHandle, b.winHandle, widthPx, heightPx, scale)
		} else {
			b.gpu.resize(widthPx, heightPx, float64(scale))
		}
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
	b.linkStarted = true
	b.handler.Frame(b, &frame{b: b}, dtSeconds)
}

// Snapshot renders one frame on the CPU and returns it as RGBA8888 pixels
// (FrameWidth*FrameHeight*4, row-major), independent of the live GPU surface.
// nil if the surface has no size.
//
// Two uses, same faithful (parity-tested) rasterizer: headless tests/
// screenshots, and the live CPU present path when GPUActive is false (iOS
// Simulator, some emulators) — there the host calls this each vsync (while
// NeedsFrame) and blits the pixels itself.
func (b *Bridge) Snapshot(dtSeconds float64) []byte {
	if b.widthPx == 0 || b.heightPx == 0 {
		return nil
	}
	b.snapshotting = true
	b.linkStarted = true
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
	const touch = shell.SourceTouch // so gestures adapt to modality (drag scrolls, long-press selects)
	switch phase {
	case TouchDown:
		// Synthesize a move first so hover/hit state sees the position.
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerMove, Pos: p, Source: touch})
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerDown, Pos: p, Source: touch})
	case TouchMove:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerMove, Pos: p, Source: touch})
	case TouchUp:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerUp, Pos: p, Source: touch})
	case TouchCancel:
		b.handler.Event(b, shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: -1e6, Y: -1e6}, Source: touch})
	}
}

// Text delivers committed text input from the on-screen keyboard.
func (b *Bridge) Text(s string) { b.handler.Event(b, shell.Text{S: s}) }

// SetInsets delivers safe-area insets in physical pixels — the status bar, the
// notch, gesture bars. The keyboard is separate (SetKeyboardHeight), because it
// is a transient mode rather than a property of the hardware.
func (b *Bridge) SetInsets(topPx, rightPx, bottomPx, leftPx float32) {
	s := b.scale
	b.handler.Event(b, shell.Insets{Insets: geom.Insets{
		Top: topPx / s, Right: rightPx / s, Bottom: bottomPx / s, Left: leftPx / s,
	}})
}

// SetKeyboardHeight delivers the on-screen keyboard's height in physical pixels,
// or 0 when it is dismissed. Hosts report it from the platform's keyboard
// notifications (iOS UIKeyboardWillChangeFrame, Android's IME insets).
func (b *Bridge) SetKeyboardHeight(heightPx float32) {
	b.handler.Event(b, shell.KeyboardInset{Height: heightPx / b.scale})
}

// TextInputActive reports whether the UI wants keyboard input; the host
// shows/hides the on-screen keyboard on transitions.
//
// A field that goes through the TextInput capability says so explicitly
// (Show/Hide), and that answer wins. The older inference — the handler reports
// whether the focused widget accepts typed text — stays as the fallback,
// because a widget can take text without being a TextField, and because hosts
// built before the capability existed still rely on it.
func (b *Bridge) TextInputActive() bool {
	if b.tiActive {
		return true
	}
	if ta, ok := b.handler.(interface{ TextInputActive() bool }); ok {
		return ta.TextInputActive()
	}
	return false
}

// Accessibility: the host (Android AccessibilityNodeProvider, iOS
// UIAccessibility) refreshes the flat node tree, then reads nodes by index
// and activates by ID. Rects are physical pixels.
//
// The tree is pulled rather than pushed because that is how both native APIs
// work — they query the app when their focus moves, on their own schedule.
// The handler side of that contract is shell.A11yProvider; the accessors
// below flatten it to the scalar-only surface gomobile bind can export.

// A11yRefresh rebuilds the accessibility node tree and returns the node
// count (0 if the handler provides no semantics).
func (b *Bridge) A11yRefresh() int {
	if p, ok := b.handler.(shell.A11yProvider); ok {
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
	if p, ok := b.handler.(shell.A11yProvider); ok {
		p.A11yActivate(id)
		b.dirty.Store(true)
	}
}

// A11yHitTest returns the node ID at a physical-pixel point (explore by
// touch), or -1.
func (b *Bridge) A11yHitTest(xPx, yPx int) int {
	if p, ok := b.handler.(shell.A11yProvider); ok {
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

// Clipboard. The bridge holds a cached copy of the system pasteboard and the
// host keeps it in step in both directions: SetClipboardText in,
// TakeClipboardWrite out.
//
// A synchronous host call would also work — gomobile does bind host methods
// that return values — but reading the pasteboard is not a free query on iOS:
// since iOS 16 touching UIPasteboard.string can show the "pasted from" banner,
// and older versions prompted. ClipboardRead is cheap enough to call from
// Build, so a design that reached the pasteboard on every read would nag the
// user once per frame. Refreshing on foreground is both cheaper and quieter.
//
// Until a host wires those two, copy and paste still work within the app and
// simply do not reach other apps — which is what shipped before this comment
// existed, undocumented.

// capabilitiesChanged tells the runtime to re-read what this Bridge offers.
//
// Capabilities are wired once when the shell hands over a Window, and a mobile
// host registers its backends after Start — so without this, everything the
// host adds is read as nil on the first frame and stays nil.
func (b *Bridge) capabilitiesChanged() {
	if b.handler == nil {
		return
	}
	b.handler.Event(b, shell.CapabilitiesChanged{})
}

// SetClipboardText delivers the system pasteboard's contents. Hosts call it
// when the app comes to the foreground and whenever the pasteboard changes
// (iOS UIPasteboard.changedNotification, Android OnPrimaryClipChangedListener).
func (b *Bridge) SetClipboardText(s string) { b.clip, b.clipHas, b.clipHasSet = s, s != "", true }

// ClipboardHost reads the system pasteboard on demand, implemented by the host.
//
// Registered only where an eager cache does not work. On Android the host
// pushes the text with SetClipboardText and this stays nil, because reading a
// ClipboardManager costs nothing and prompts for nothing. On iOS it is the
// whole point: UIPasteboard.string cannot be read to fill a cache without
// showing the user a "pasted from" prompt they did not ask for.
//
// Runs on the UI thread and must return promptly.
type ClipboardHost interface {
	// ClipboardText returns the pasteboard's text, or "" when it holds none.
	ClipboardText() string
}

// SetClipboardHost registers the pasteboard reader. Call once during setup.
func (b *Bridge) SetClipboardHost(h ClipboardHost) { b.clipHost = h }

// SetClipboardHasText reports whether the pasteboard holds text.
//
// Split from SetClipboardText so a host can answer the question the edit menu
// actually asks without paying to read. iOS has UIPasteboard.hasStrings for
// exactly this, and it raises no prompt; a host that pushes the text instead
// answers both at once and need not call this.
func (b *Bridge) SetClipboardHasText(v bool) { b.clipHas, b.clipHasSet = v, true }

// ClipboardHasText makes the Bridge a widget.ClipboardPeeker.
//
// Falls back to the cache when no host has answered, so a host wired the old
// way — pushing text and nothing else — keeps working unchanged.
func (b *Bridge) ClipboardHasText() bool {
	if b.clipHasSet {
		return b.clipHas
	}
	return b.clip != ""
}

// TakeClipboardWrite returns text the app has copied and the host has not yet
// written to the system pasteboard, or "" when there is nothing pending. Hosts
// drain it next to TakeOpenedURL and write the result to UIPasteboard /
// ClipboardManager.
//
// Copying the empty string is indistinguishable from nothing pending. gomobile
// allows a second result only when it is an error, so the alternative is a
// second bound method to report pending-ness; clearing the pasteboard is not a
// thing apps do, so the ambiguity is inert and one method is the better trade.
func (b *Bridge) TakeClipboardWrite() string {
	if !b.clipPending {
		return ""
	}
	b.clipPending = false
	return b.clip
}

// shell.Window implementation.

func (b *Bridge) Invalidate()     { b.dirty.Store(true) }
func (b *Bridge) SetTitle(string) {}
func (b *Bridge) Close()          {}
func (b *Bridge) DarkMode() bool  { return b.dark }
func (b *Bridge) OpenURL(u string) error {
	b.opened = append(b.opened, u)
	return nil
}
func (b *Bridge) ClipboardRead() (string, error) {
	// Through the host where one is registered, because the cache cannot be
	// kept fresh on iOS without prompting: the only way to know what the
	// pasteboard holds is to read it, and reading is what raises the prompt.
	// Callers reach this from an explicit paste, where a prompt is the expected
	// behaviour rather than a surprise; the edit menu asks ClipboardHasText.
	if b.clipHost != nil {
		return b.clipHost.ClipboardText(), nil
	}
	return b.clip, nil
}
func (b *Bridge) ClipboardWrite(s string) error {
	// The peek answer moves with the write, or a copy inside the app leaves it
	// stale: the host's last push said "no text", and the edit menu would go on
	// hiding Paste for something the user just copied.
	b.clip, b.clipPending = s, true
	b.clipHas, b.clipHasSet = s != "", true
	return nil
}

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
	if f.b.capturing != nil && f.b.gpu != nil {
		return f.b.capturing
	}
	if !f.b.snapshotting && f.b.gpu != nil {
		return mobileGPUTarget{f.b.gpu}
	}
	// The damage rect is ignored: the host reads the whole frame back through
	// Snapshot and blits it, so there is no retained destination here to
	// partially update.
	return shell.PixelTarget{Put: func(img *image.RGBA, _ geom.Rect) { f.b.frame = img }}
}

// gpuCaptureTarget is the offscreen render target CaptureGPU installs. It is
// an interface so the field can exist on every platform while the
// implementation stays behind the build tag that has a GPU.
type gpuCaptureTarget interface{ shell.Target }
