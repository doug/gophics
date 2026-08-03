package app

import (
	"testing"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// TestInputHeldStateHeadless drives keys through the real Core dispatch and
// checks held-state polling + per-frame edge clearing, focus-free.
func TestInputHeldStateHeadless(t *testing.T) {
	h, err := NewHeadless(widget.Canvas{W: 100, H: 100}, Config{Size: geom.Size{W: 200, H: 200}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	in := h.Core.Owner.Input
	if in == nil {
		t.Fatal("Owner.Input is nil")
	}

	h.KeyDown(shell.KeyW)
	if !in.Down(shell.KeyW) || !in.JustPressed(shell.KeyW) {
		t.Fatal("after KeyDown: expected Down + JustPressed")
	}
	h.Step(0.016) // NewFrame clears edges
	if !in.Down(shell.KeyW) {
		t.Fatal("key should stay held across frames")
	}
	if in.JustPressed(shell.KeyW) {
		t.Fatal("JustPressed should clear after the frame")
	}

	h.KeyDown(shell.KeyRight)
	if in.Axis(shell.KeyLeft, shell.KeyRight) != 1 {
		t.Fatal("Axis(Left,Right) should be +1 with Right held")
	}

	h.KeyUp(shell.KeyW)
	if in.Down(shell.KeyW) || !in.JustReleased(shell.KeyW) {
		t.Fatal("after KeyUp: not Down, JustReleased")
	}
	h.Step(0.016)
	if in.JustReleased(shell.KeyW) {
		t.Fatal("JustReleased should clear after the frame")
	}
}
