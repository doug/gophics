package app

import (
	"image"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// Headless drives an app without a display: the widget-test harness.
// Drive input with Tap/Move/Type/Key, then Render to inspect pixels — or
// assert on state directly.
type Headless struct {
	Core *Core
	// OpenedURLs records ctx.OpenURL calls for assertions.
	OpenedURLs []string

	size  geom.Size
	scale float32
}

// NewHeadless builds a headless app at the given logical size and scale.
func NewHeadless(root widget.Widget, cfg Config, scale float32) (*Headless, error) {
	core, err := NewCore(root, cfg)
	if err != nil {
		return nil, err
	}
	core.Owner.RequestFrame = func() {} // frames are pulled via Render
	core.Owner.Clipboard = &MemClipboard{}
	h := &Headless{Core: core, size: cfg.Size, scale: scale}
	core.Owner.OpenURL = func(url string) error {
		h.OpenedURLs = append(h.OpenedURLs, url)
		return nil
	}
	core.mount() // hooks wired above; safe to mount (may launch Posters)
	return h, nil
}

// MemClipboard is the in-memory clipboard used by Headless.
type MemClipboard struct{ S string }

func (m *MemClipboard) ClipboardRead() (string, error)  { return m.S, nil }
func (m *MemClipboard) ClipboardWrite(s string) error   { m.S = s; return nil }

// Render lays out and paints a frame through the damage-aware pipeline,
// returning the physical-pixel image (retained across frames, so an
// unchanged scene skips rasterization — check Core.Skipped).
func (h *Headless) Render() image.Image {
	h.Core.drainPosted()
	h.Core.Layout(h.size)
	if changed, damage := h.Core.RecordScene(h.size, h.scale); changed {
		c := h.Core.Painter.BeginOffscreen(h.size, h.scale)
		h.Core.ReplayDamaged(c, damage)
	}
	return h.Core.Painter.Image()
}

// SetDarkMode sets the simulated platform color scheme and rebuilds.
func (h *Headless) SetDarkMode(dark bool) {
	h.Core.Owner.DarkMode = dark
	h.Core.Owner.RebuildAll()
}

// Step advances animations by dt seconds, reporting whether any are still
// running. Deterministic replacement for vsync in tests.
func (h *Headless) Step(dt float64) bool { return h.Core.Owner.TickAll(dt) }

// Drag dispatches press at from, a move to to (exceeding tap slop), and
// release at to.
func (h *Headless) Drag(from, to geom.Pt) {
	h.layoutForInput()
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: from})
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: to})
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: to})
}

// Scroll dispatches a scroll delta at the last pointer position (Move first
// to position the pointer).
func (h *Headless) Scroll(delta geom.Pt) {
	h.layoutForInput()
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerScroll, Scroll: delta})
}

// Move dispatches a pointer move (hover) to p.
func (h *Headless) Move(p geom.Pt) {
	h.layoutForInput()
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p})
}

// Tap dispatches a press+release at p.
func (h *Headless) Tap(p geom.Pt) {
	h.layoutForInput()
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: p})
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
}

// Type dispatches committed text input. Pending rebuilds are flushed
// first, mirroring the frame between events in a real shell.
func (h *Headless) Type(s string) {
	h.layoutForInput()
	h.Core.Keyboard(shell.Text{S: s})
}

// Key dispatches a key press (flushing pending rebuilds first).
func (h *Headless) Key(code shell.KeyCode) {
	h.layoutForInput()
	h.Core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code})
}

// Compose dispatches an IME composition update (Start on first call).
func (h *Headless) Compose(preedit string, cursor int) {
	h.layoutForInput()
	h.Core.Keyboard(shell.Composition{Kind: shell.CompositionUpdate, Preedit: preedit, Cursor: cursor})
}

// CommitComposition ends composition, committing s.
func (h *Headless) CommitComposition(s string) {
	h.layoutForInput()
	h.Core.Keyboard(shell.Composition{Kind: shell.CompositionEnd, Committed: s})
}

// KeyMod dispatches a key press with modifiers.
func (h *Headless) KeyMod(code shell.KeyCode, mods shell.Mods) {
	h.layoutForInput()
	h.Core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code, Mods: mods})
}

// DragTo dispatches press at from and a move to to without releasing
// (for selection dragging; call Release to finish).
func (h *Headless) DragTo(from, to geom.Pt) {
	h.layoutForInput()
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: from})
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: to})
}

// Release dispatches pointer-up at p.
func (h *Headless) Release(p geom.Pt) {
	h.Core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: p})
}

// layoutForInput ensures hit testing sees current sizes even before the
// first Render.
func (h *Headless) layoutForInput() {
	h.Core.Layout(h.size)
}
