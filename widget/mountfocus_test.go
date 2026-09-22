package widget

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// Mounting a text field focuses nothing. A focused field raises the soft
// keyboard, so the old rule — "a focusable widget mounted while nothing has
// focus takes it" — opened every page containing a field with the keyboard
// covering half the screen, and the raise cancelled whatever touch gesture
// was in progress. No platform focuses a field the user has not tapped.
func TestMountingAFieldDoesNotFocusIt(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(TextField{Value: "hi"})
	o.FlushBuilds()

	if o.KeyboardTarget != nil {
		t.Error("a field took focus on mount; nobody tapped it")
	}
	if ti.shown != 0 {
		t.Errorf("the keyboard was raised %d times by a page merely containing a field", ti.shown)
	}
}

// Key-only widgets keep the implicit rule. A canvas that reads arrows or a menu
// that reads Escape has to work from the first frame, and focusing one is
// invisible: nothing raises a keyboard for a widget with no OnText.
func TestKeyOnlyWidgetStillTakesFocusOnMount(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(Interactive{
		Gestures: Gestures{OnKey: func(shell.Key) {}},
		Child:    Sized{W: 10, H: 10},
	})
	o.FlushBuilds()

	if o.KeyboardTarget == nil {
		t.Fatal("a key-only widget mounted with nothing focused did not take focus; " +
			"its arrows would go nowhere until the user clicked it")
	}
	if ti.shown != 0 {
		t.Errorf("focusing a key-only widget raised the keyboard %d times", ti.shown)
	}
}

// Autofocus is how a screen says which field it opens into. It is the one way
// a field starts focused, and it goes through the same focus callback a tap
// does, so the keyboard comes up with it.
func TestAutofocusFieldTakesFocusAndRaisesTheKeyboard(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(TextField{Value: "hi", Autofocus: true})
	o.FlushBuilds()

	if o.KeyboardTarget == nil {
		t.Fatal("an Autofocus field did not take focus on mount")
	}
	if ti.shown != 1 {
		t.Errorf("the keyboard was raised %d times for an Autofocus field, want 1", ti.shown)
	}
}

// Both mount-time rules are spent at mount whether or not focus was taken. The
// second field on a screen finds the first holding focus and declines; it must
// not then be waiting to grab focus the moment the first releases it, or
// dismissing the keyboard lasts one frame before a field nobody touched
// brings it straight back.
func TestSecondFieldDoesNotStealReleasedFocus(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(Column(
		TextField{Value: "first", Autofocus: true},
		TextField{Value: "second"},
	))
	o.FlushBuilds()

	first := o.KeyboardTarget
	if first == nil {
		t.Fatal("the Autofocus field did not take focus")
	}

	// Release the way a tap on empty space does: clear the target and tell
	// the field it lost focus, which puts the keyboard away.
	o.KeyboardTarget = nil
	first.OnFocus(false)
	if ti.hidden != 1 {
		t.Fatalf("blur hid the keyboard %d times, want 1", ti.hidden)
	}

	// Several frames, each of which rebuilds and re-runs the mount rules.
	for range 5 {
		o.RebuildAll()
		o.FlushBuilds()
		if got := o.KeyboardTarget; got != nil {
			which := "the first field"
			if got != first {
				which = "the second field"
			}
			t.Fatalf("%s re-took focus after it was released; the keyboard would come straight back up", which)
		}
	}
	if ti.shown != 1 {
		t.Errorf("the keyboard was raised %d times in total, want the one from Autofocus", ti.shown)
	}
}
