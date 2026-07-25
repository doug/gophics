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
	Core  *Core
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
	return &Headless{Core: core, size: cfg.Size, scale: scale}, nil
}

// Render lays out and paints a frame through the damage-aware pipeline,
// returning the physical-pixel image (retained across frames, so an
// unchanged scene skips rasterization — check Core.Skipped).
func (h *Headless) Render() image.Image {
	h.Core.Layout(h.size)
	if changed, damage := h.Core.RecordScene(h.size, h.scale); changed {
		c := h.Core.Painter.BeginOffscreen(h.size, h.scale)
		h.Core.ReplayDamaged(c, damage)
	}
	return h.Core.Painter.Image()
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

// Type dispatches committed text input.
func (h *Headless) Type(s string) {
	h.Core.Keyboard(shell.Text{S: s})
}

// Key dispatches a key press.
func (h *Headless) Key(code shell.KeyCode) {
	h.Core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: code})
}

// layoutForInput ensures hit testing sees current sizes even before the
// first Render.
func (h *Headless) layoutForInput() {
	h.Core.Layout(h.size)
}
