package anim

import (
	"testing"
	"time"
)

func TestControllerForwardCompletes(t *testing.T) {
	changes := 0
	c := &Controller{Duration: 100 * time.Millisecond, Curve: Linear, OnChange: func() { changes++ }}
	if c.Tick(0.016) {
		t.Fatal("idle controller must not tick")
	}
	c.Forward()
	if !c.Running() {
		t.Fatal("Forward should start the controller")
	}
	for i := 0; i < 20 && c.Tick(0.016); i++ {
	}
	if c.Running() || c.Value() != 1 {
		t.Fatalf("controller should complete at 1, got %v (running=%v)", c.Value(), c.Running())
	}
	if changes == 0 {
		t.Fatal("OnChange never fired")
	}
}

func TestControllerToggleReverses(t *testing.T) {
	c := &Controller{Duration: 100 * time.Millisecond, Curve: Linear}
	c.Forward()
	c.Tick(0.05) // halfway
	if v := c.Value(); v < 0.4 || v > 0.6 {
		t.Fatalf("halfway value = %v", v)
	}
	c.Toggle() // now reversing
	for i := 0; i < 20 && c.Tick(0.016); i++ {
	}
	if c.Value() != 0 {
		t.Fatalf("reverse should land at 0, got %v", c.Value())
	}
	c.Toggle()
	if !c.Running() {
		t.Fatal("toggle from 0 should run forward")
	}
}

func TestCurveEndpoints(t *testing.T) {
	for _, curve := range []Curve{Linear, EaseIn, EaseOut, EaseInOut} {
		if got := curve(0); got != 0 {
			t.Fatalf("curve(0) = %v", got)
		}
		if got := curve(1); got != 1 {
			t.Fatalf("curve(1) = %v", got)
		}
	}
}

func TestJump(t *testing.T) {
	c := &Controller{Curve: Linear}
	c.Jump(1)
	if c.Value() != 1 || c.Running() {
		t.Fatalf("Jump(1): value=%v running=%v", c.Value(), c.Running())
	}
}
