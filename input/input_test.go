package input

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func press(s *State, k shell.KeyCode)   { s.HandleKey(shell.Key{Kind: shell.KeyPress, Code: k}) }
func release(s *State, k shell.KeyCode) { s.HandleKey(shell.Key{Kind: shell.KeyRelease, Code: k}) }

func TestHeldAndEdges(t *testing.T) {
	s := New()
	press(s, shell.KeyW)
	if !s.Down(shell.KeyW) || !s.JustPressed(shell.KeyW) || s.JustReleased(shell.KeyW) {
		t.Fatal("press: expected Down + JustPressed, not JustReleased")
	}
	s.NewFrame()
	if !s.Down(shell.KeyW) || s.JustPressed(shell.KeyW) {
		t.Fatal("after NewFrame: still held, edge cleared")
	}
	release(s, shell.KeyW)
	if s.Down(shell.KeyW) || !s.JustReleased(shell.KeyW) {
		t.Fatal("release: not Down, JustReleased")
	}
}

func TestSameFrameTap(t *testing.T) {
	s := New()
	press(s, shell.KeySpace)
	release(s, shell.KeySpace)
	if !s.JustPressed(shell.KeySpace) || !s.JustReleased(shell.KeySpace) {
		t.Fatal("a same-frame tap must register both edges (sticky)")
	}
	if s.Down(shell.KeySpace) {
		t.Fatal("not held after the tap")
	}
}

func TestAxis(t *testing.T) {
	s := New()
	press(s, shell.KeyD)
	if s.Axis(shell.KeyA, shell.KeyD) != 1 {
		t.Fatal("D held → +1")
	}
	press(s, shell.KeyA)
	if s.Axis(shell.KeyA, shell.KeyD) != 0 {
		t.Fatal("both held → 0")
	}
	release(s, shell.KeyD)
	if s.Axis(shell.KeyA, shell.KeyD) != -1 {
		t.Fatal("A held → -1")
	}
}

func TestClearOnBlur(t *testing.T) {
	s := New()
	press(s, shell.KeyW)
	s.Clear()
	if s.Down(shell.KeyW) || s.JustPressed(shell.KeyW) {
		t.Fatal("Clear should drop all held state")
	}
}

func TestPointer(t *testing.T) {
	s := New()
	s.HandlePointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 10, Y: 20}, Button: 0})
	if !s.PointerDown(0) || !s.PointerJustPressed(0) || s.Pointer() != (geom.Pt{X: 10, Y: 20}) {
		t.Fatal("pointer down: held + edge + position")
	}
	s.NewFrame()
	if !s.PointerDown(0) || s.PointerJustPressed(0) {
		t.Fatal("after NewFrame: still held, edge cleared")
	}
	s.HandlePointer(shell.Pointer{Kind: shell.PointerUp, Button: 0})
	if s.PointerDown(0) {
		t.Fatal("pointer up: released")
	}
}
