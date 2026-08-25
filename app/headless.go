package app

import (
	"image"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/scene"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Headless drives an app without a display: the widget-test harness.
// Drive input with Tap/Move/Type/Key, then Render to inspect pixels — or
// assert on state directly.
type Headless struct {
	core *core
	// OpenedURLs records ctx.OpenURL calls for assertions.
	OpenedURLs []string

	size  geom.Size
	scale float32

	// gpu holds the lazily-created headless GPU renderer used by RenderGPU
	// (gophics_gpu build only); typed any so this file stays tag-agnostic.
	gpu any

	// lastImage caches the surface returned by Render. Painter.Image() copies
	// the whole backing pixmap into a fresh image.RGBA, so re-fetching it on a
	// frame that skipped rasterization (nothing changed) would pay that copy
	// for pixels that are byte-for-byte what was already returned.
	lastImage image.Image
}

// NewHeadless builds a headless app at the given logical size and scale.
func NewHeadless(root widget.Widget, cfg Config, scale float32) (*Headless, error) {
	core, err := newCore(root, cfg)
	if err != nil {
		return nil, err
	}
	core.Owner.RequestFrame = func() {} // frames are pulled via Render
	core.Owner.Clipboard = &MemClipboard{}
	h := &Headless{core: core, size: cfg.Size, scale: scale}
	core.Owner.OpenURL = func(url string) error {
		h.OpenedURLs = append(h.OpenedURLs, url)
		return nil
	}
	core.mount() // hooks wired above; safe to mount (may launch Posters)
	return h, nil
}

// MemClipboard is the in-memory clipboard used by Headless.
type MemClipboard struct{ S string }

func (m *MemClipboard) ClipboardRead() (string, error) { return m.S, nil }
func (m *MemClipboard) ClipboardWrite(s string) error  { m.S = s; return nil }

// Render lays out and paints a frame through the damage-aware pipeline,
// returning the physical-pixel image (retained across frames, so an
// unchanged scene skips rasterization — check core.Skipped).
func (h *Headless) Render() image.Image {
	h.core.drainPosted()
	h.core.Layout(h.size)
	if changed, damage := h.core.RecordScene(h.size, h.scale); changed || h.lastImage == nil {
		c := h.core.Painter.BeginOffscreen(h.size, h.scale)
		h.core.ReplayDamaged(c, damage)
		h.lastImage = h.core.Painter.Image()
	}
	return h.lastImage
}

// SetDarkMode sets the simulated platform color scheme and rebuilds.
func (h *Headless) SetDarkMode(dark bool) {
	h.core.Owner.DarkMode = dark
	h.core.Owner.RebuildAll()
}

// SetSafeInsets reports the platform-obstructed edges — the notch, the status
// bar, the home indicator — that a real device would.
//
// Headless they are zero, which is also what every desktop reports, so a
// layout that hides its title under the Dynamic Island looks perfectly correct
// in every test until it reaches a phone. This is how a test says "pretend
// this is that phone".
func (h *Headless) SetSafeInsets(in geom.Insets) {
	h.core.Owner.SafeInsets = in
	h.core.Owner.RebuildAll()
}

// Resize changes the logical surface size (simulates a window resize) and
// delivers a Resize event so the tree can react.
func (h *Headless) Resize(size geom.Size) {
	h.size = size
	h.core.Owner.RebuildAll()
}

// Step advances animations by dt seconds, reporting whether any are still
// running. Deterministic replacement for vsync in tests.
func (h *Headless) Step(dt float64) bool {
	h.core.TickGestures(dt)
	r := h.core.Owner.TickAll(dt)
	if h.core.Owner.Input != nil {
		h.core.Owner.Input.NewFrame() // clear per-frame edges after tickers read them
	}
	return r
}

// Drag dispatches press at from, a move to to (exceeding tap slop), and
// release at to.
func (h *Headless) Drag(from, to geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: from})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: to})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: to})
}

// Scroll dispatches a scroll delta at the last pointer position (Move first
// to position the pointer).
func (h *Headless) Scroll(delta geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerScroll, Scroll: delta})
}

// Move dispatches a pointer move (hover) to p.
func (h *Headless) Move(p geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
}

// Tap dispatches a press+release at p.
func (h *Headless) Tap(p geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: p})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
}

// Type dispatches committed text input. Pending rebuilds are flushed
// first, mirroring the frame between events in a real shell.
func (h *Headless) Type(s string) {
	h.layoutForInput()
	h.core.Keyboard(shell.Text{S: s})
}

// Key dispatches a full key tap — press then release — flushing pending
// rebuilds first. For held-key tests (Ctx.Input() polling) use KeyDown/KeyUp.
func (h *Headless) Key(code shell.KeyCode) {
	h.layoutForInput()
	h.core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code})
	h.core.Keyboard(shell.Key{Kind: shell.KeyRelease, Code: code})
}

// KeyDown dispatches a key-press and holds it. Pair with KeyUp to test
// held-state polling via Ctx.Input().
func (h *Headless) KeyDown(code shell.KeyCode) {
	h.layoutForInput()
	h.core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code})
}

// KeyUp dispatches a key-release.
func (h *Headless) KeyUp(code shell.KeyCode) {
	h.layoutForInput()
	h.core.Keyboard(shell.Key{Kind: shell.KeyRelease, Code: code})
}

// Compose dispatches an IME composition update (Start on first call).
func (h *Headless) Compose(preedit string, cursor int) {
	h.layoutForInput()
	h.core.Keyboard(shell.Composition{Kind: shell.CompositionUpdate, Preedit: preedit, Cursor: cursor})
}

// CommitComposition ends composition, committing s.
func (h *Headless) CommitComposition(s string) {
	h.layoutForInput()
	h.core.Keyboard(shell.Composition{Kind: shell.CompositionEnd, Committed: s})
}

// KeyMod dispatches a key press with modifiers.
func (h *Headless) KeyMod(code shell.KeyCode, mods shell.Mods) {
	h.layoutForInput()
	h.core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code, Mods: mods})
}

// DragTo dispatches press at from and a move to to without releasing
// (for selection dragging; call Release to finish).
func (h *Headless) DragTo(from, to geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: from})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: to})
}

// TouchDrag is Drag from a touch device rather than a mouse.
//
// The distinction is not cosmetic: gesture arbitration asks whether a gesture
// came from touch, and handlers answer differently — a drag that selects text
// on a mouse scrolls on a finger, and a scroller claims a drag it would leave
// to the wheel. A harness that only ever sends mouse events cannot see any of
// that, which is how touch-only faults reach a device untested.
func (h *Headless) TouchDrag(from, to geom.Pt) {
	h.TouchPress(from)
	h.TouchMove(to)
	h.TouchRelease(to)
}

// TouchPress dispatches a touch-sourced pointer-down at p without releasing.
func (h *Headless) TouchPress(p geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: p, Source: shell.SourceTouch})
}

// TouchMove dispatches a touch-sourced move to p.
func (h *Headless) TouchMove(p geom.Pt) {
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p, Source: shell.SourceTouch})
}

// TouchRelease dispatches a touch-sourced pointer-up at p.
func (h *Headless) TouchRelease(p geom.Pt) {
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p, Source: shell.SourceTouch})
}

// Owner returns the widget Owner: the tree's shared services — state
// snapshot/restore (hot-restart tests), clipboard, semantics. For tests that
// assert past the input/render surface.
func (h *Headless) Owner() *widget.Owner { return h.core.Owner }

// Scene returns the display list recorded for the last frame, for tests that
// need to assert on what was drawn rather than on what it looked like.
//
// It is prev, not cur: recordScene swaps the two once it has diffed them, so
// the frame just recorded is the one now called previous.
func (h *Headless) Scene() *scene.List { return h.core.prev }

// Semantics returns the flattened accessibility tree for assertions.
func (h *Headless) Semantics() []layout.SemNode { return h.core.Semantics() }

// Press dispatches pointer-down at p without releasing — for testing
// press-and-hold feedback (pressed highlights, long-press). Pair with Release.
func (h *Headless) Press(p geom.Pt) {
	h.layoutForInput()
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: p})
}

// Release dispatches pointer-up at p.
func (h *Headless) Release(p geom.Pt) {
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
}

// layoutForInput ensures hit testing sees current sizes even before the
// first Render.
func (h *Headless) layoutForInput() {
	h.core.Layout(h.size)
}
