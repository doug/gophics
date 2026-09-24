package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// gestureCounts records every gesture callback a draggable box receives.
type gestureCounts struct {
	press, pressEnd, drag, release, tap int
}

// draggableApp is one full-window box that presses, drags and releases.
type draggableApp struct{ n *gestureCounts }

func (a draggableApp) Build(widget.Ctx) widget.Widget {
	n := a.n
	return widget.Interactive{
		Gestures: widget.Gestures{
			OnPress:    func(geom.Pt) { n.press++ },
			OnPressEnd: func() { n.pressEnd++ },
			OnDrag:     func(geom.Pt, geom.Pt) { n.drag++ },
			OnRelease:  func() { n.release++ },
			OnTap:      func() { n.tap++ },
		},
		Child: widget.Sized{W: 200, H: 200},
	}
}

func newDraggable(t *testing.T) (*Headless, *gestureCounts) {
	t.Helper()
	n := &gestureCounts{}
	h, err := NewHeadless(draggableApp{n}, Config{Size: geom.Size{W: 200, H: 200}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, n
}

// A pointer-down arriving while a drag is live — a second finger on a touch
// screen — ends the drag properly: OnRelease for the dragging box, OnPressEnd
// for the pressed ones. The lists used to be reset silently, which left a
// scroller in its dragging state with no fling and highlights stuck.
func TestSecondPointerDownEndsTheLiveGesture(t *testing.T) {
	h, n := newDraggable(t)
	h.Press(geom.Pt{X: 50, Y: 50})
	h.Move(geom.Pt{X: 90, Y: 50}) // past slop: the drag commits
	if n.drag == 0 {
		t.Fatal("drag never started")
	}
	h.Press(geom.Pt{X: 120, Y: 120})
	if n.release != 1 {
		t.Errorf("OnRelease fired %d times when a second down interrupted the drag, want 1", n.release)
	}
	if n.pressEnd != 1 {
		t.Errorf("OnPressEnd fired %d times for the interrupted press, want 1", n.pressEnd)
	}
	h.Release(geom.Pt{X: 120, Y: 120})
	h.Release(geom.Pt{X: 90, Y: 50})
	if n.press != 2 || n.pressEnd != 2 {
		t.Errorf("press=%d pressEnd=%d after both fingers lifted, want 2 and 2", n.press, n.pressEnd)
	}
	if n.release != 1 {
		t.Errorf("OnRelease fired %d times in total, want 1: the second press never dragged", n.release)
	}
}

// A cancel ends the gesture like an up would, minus the tap, and does not
// move the polled pointer. Focus loss used to synthesize an up at
// (-1e6, -1e6), which Input stored as the pointer position and, being
// sourced from nothing, flipped PointerIsTouch back to mouse.
func TestPointerCancelEndsGestureWithoutMovingPointer(t *testing.T) {
	h, n := newDraggable(t)
	in := h.Owner().Input
	h.TouchPress(geom.Pt{X: 50, Y: 50})
	h.TouchMove(geom.Pt{X: 90, Y: 50})
	if n.drag == 0 {
		t.Fatal("drag never started")
	}
	h.core.Pointer(shell.Pointer{Kind: shell.PointerCancel, Source: shell.SourceTouch})
	if n.release != 1 || n.pressEnd != 1 {
		t.Errorf("after cancel: release=%d pressEnd=%d, want 1 and 1", n.release, n.pressEnd)
	}
	if n.tap != 0 {
		t.Error("a cancelled press produced a tap")
	}
	if got := in.Pointer(); got != (geom.Pt{X: 90, Y: 50}) {
		t.Errorf("Pointer() = %v after cancel, want the last real position (90,50)", got)
	}
	if !in.PointerIsTouch() {
		t.Error("PointerIsTouch flipped to mouse on a cancel")
	}
	if in.PointerDown(0) {
		t.Error("the primary button is still reported held after a cancel")
	}
	// The next press starts clean: no leftover release for a drag that ended.
	h.Tap(geom.Pt{X: 20, Y: 20})
	if n.tap != 1 || n.release != 1 {
		t.Errorf("after a fresh tap: tap=%d release=%d, want 1 and 1", n.tap, n.release)
	}
}

// A cancelled tap — down, then cancel before up — fires no OnTap.
func TestPointerCancelDropsTheTap(t *testing.T) {
	h, n := newDraggable(t)
	h.Press(geom.Pt{X: 50, Y: 50})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerCancel})
	if n.tap != 0 || n.pressEnd != 1 {
		t.Errorf("tap=%d pressEnd=%d after a cancelled press, want 0 and 1", n.tap, n.pressEnd)
	}
}

// Losing window focus mid-drag goes through the same cancel path, via the
// handler's Event.
func TestFocusLossCancelsWithoutMovingPointer(t *testing.T) {
	n := &gestureCounts{}
	h, err := NewHandler(draggableApp{n}, Config{Size: geom.Size{W: 200, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	sh := h.(*shellHandler)
	w := &fakeWindow{}
	f := &fakeFrame{size: geom.Size{W: 200, H: 200}, scale: 1,
		tgt: shell.PixelTarget{Put: func(*image.RGBA, geom.Rect) {}}}
	sh.Frame(w, f, 0)
	sh.Event(w, shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 50, Y: 50}})
	sh.Event(w, shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: 90, Y: 50}})
	sh.Event(w, shell.Focus{Focused: false})
	if n.release != 1 || n.pressEnd != 1 {
		t.Errorf("after focus loss: release=%d pressEnd=%d, want 1 and 1", n.release, n.pressEnd)
	}
	in := sh.core.Owner.Input
	if got := in.Pointer(); got != (geom.Pt{X: 90, Y: 50}) {
		t.Errorf("Pointer() = %v after focus loss, want (90,50)", got)
	}
	if in.PointerDown(0) {
		t.Error("primary button still held after focus loss")
	}
}

// fieldAndSpaceApp is a text field under an empty area, so a tap can land in
// the field or outside anything focusable.
type fieldAndSpaceApp struct{}

func (fieldAndSpaceApp) Build(widget.Ctx) widget.Widget {
	return widget.Column(widget.Sized{W: 200, H: 80}, widget.TextField{Autofocus: true})
}

// Input.TextCapturing follows focus as it moves by tap, not only by key. It
// was refreshed inside Keyboard only, so after tapping into a field it read
// false until the next key event, and after tapping out it stayed true — a
// game holding a movement key kept moving into the chat, then ignored the
// keys after the player left it.
func TestTextCapturingFollowsTapFocus(t *testing.T) {
	h, err := NewHeadless(fieldAndSpaceApp{}, Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	var field geom.Rect
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Role == layout.RoleTextField {
			field = n.Rect
		}
	}
	if field.IsEmpty() {
		t.Fatal("no text field in the semantics tree")
	}
	in := h.Owner().Input
	// Autofocus took focus at mount, during a build: the poll must already
	// say so before any event arrives.
	if h.Owner().KeyboardTarget == nil {
		t.Fatal("Autofocus did not focus the field at mount")
	}
	if !in.TextCapturing() {
		t.Error("TextCapturing false after a field took focus at mount")
	}
	h.Tap(geom.Pt{X: 20, Y: 20}) // outside anything focusable: releases the field
	if h.Owner().KeyboardTarget != nil {
		t.Fatal("tapping outside did not release the field")
	}
	if in.TextCapturing() {
		t.Error("TextCapturing still true after tapping out of the field")
	}
	h.Tap(geom.Pt{X: (field.Min.X + field.Max.X) / 2, Y: (field.Min.Y + field.Max.Y) / 2})
	if t0 := h.Owner().KeyboardTarget; t0 == nil || t0.OnText == nil {
		t.Fatal("tapping the field did not focus it")
	}
	if !in.TextCapturing() {
		t.Error("TextCapturing false after tapping into the field, before any key")
	}
}
