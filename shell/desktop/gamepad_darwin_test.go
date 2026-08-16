//go:build darwin && !ios && !js

package desktop

import (
	"testing"

	"github.com/doug/gophics/internal/objc"
)

// Poll must be safe and empty with nothing attached — a game calls it every
// frame, so loading GameController must not panic, block or pop UI.
func TestDarwinPollWithNoController(t *testing.T) {
	g := darwinGamepads{}
	for range 3 {
		got := g.Poll()
		for _, pad := range got {
			// If a controller is attached the shape still has to be right.
			if pad.ID == "" {
				t.Error("connected controller reported an empty ID")
			}
			if len(pad.Buttons) != len(buttonSelectors)+len(dpadSelectors) {
				t.Errorf("got %d buttons, want %d", len(pad.Buttons),
					len(buttonSelectors)+len(dpadSelectors))
			}
			if len(pad.Axes) != 4 {
				t.Errorf("got %d axes, want 4", len(pad.Axes))
			}
		}
	}
}

// SendFloat is new, and a wrong return width reads as garbage rather than as a
// crash — exactly the failure that looks like a working driver reporting
// nonsense stick values. NSNumber provides a known float to check against.
func TestSendFloatReadsKnownValues(t *testing.T) {
	cls := objc.Class("NSNumber")
	if !cls.Valid() {
		t.Skip("NSNumber unavailable")
	}
	for _, want := range []int64{0, 1, 42, -7} {
		n := cls.Send("numberWithInteger:", objc.Int(want))
		if !n.Valid() {
			t.Fatal("could not create NSNumber")
		}
		if got := n.SendFloat("floatValue"); got != float32(want) {
			t.Errorf("SendFloat(floatValue) = %v, want %v", got, float32(want))
		}
	}
}

// Messaging nil is how an absent button reports itself, so it has to answer
// zero rather than panicking or returning stack garbage.
func TestSendFloatOnNilIsZero(t *testing.T) {
	if got := objc.ID(0).SendFloat("value"); got != 0 {
		t.Errorf("SendFloat on nil = %v, want 0", got)
	}
}
