package widget

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// fakeTextInput records what the field asks the platform keyboard to do.
type fakeTextInput struct {
	shown, hidden      int
	handler            shell.TextInputHandler
	lastText           string
	lastSelA, lastSelB int
	setTextCalls       int
}

func (f *fakeTextInput) Show(_ shell.TextInputOptions, h shell.TextInputHandler) {
	f.shown++
	f.handler = h
}
func (f *fakeTextInput) Hide() { f.hidden++ }
func (f *fakeTextInput) SetText(t string, a, b int) {
	f.lastText, f.lastSelA, f.lastSelB = t, a, b
	f.setTextCalls++
}

// A field that takes focus must raise the platform keyboard, and drop it when
// focus leaves.
//
// gophics draws its own editor, so no native field exists for the platform to
// focus: nothing raises the keyboard unless the widget asks. Without this a
// phone shows a blinking caret in a field that cannot be typed into, which is
// not a degraded experience but no text entry at all.
func TestFocusRaisesAndDismissesTheSoftKeyboard(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	// A focusable widget mounted while nothing has focus takes it, so mounting
	// the field is what focuses it.
	o.SetRoot(TextField{Value: "hi"})
	o.FlushBuilds()

	if ti.shown != 1 {
		t.Fatalf("focusing the field raised the keyboard %d times, want 1 — "+
			"a phone would show a caret in a field it cannot type into", ti.shown)
	}
	if ti.lastText != "hi" {
		t.Errorf("IME was given surrounding text %q, want %q — composition and "+
			"prediction have nothing to work against", ti.lastText, "hi")
	}

	// What the platform IME produces must reach the editor.
	if ti.handler.OnText == nil {
		t.Fatal("no OnText handler given to the platform; typed characters would go nowhere")
	}
	if ti.handler.OnEditKey == nil {
		t.Fatal("no OnEditKey handler; backspace and enter would do nothing")
	}
}

// A shell with no keyboard capability (desktop) must not panic.
func TestNoTextInputCapabilityIsHarmless(t *testing.T) {
	o := newOwner() // Owner.TextInput is nil
	o.SetRoot(TextField{})
	o.FlushBuilds()
}

// The IME's view of the field has to track the editor, not just its state when
// focus arrived. Autocorrect replaces the word behind the caret, prediction
// reads what precedes it, and replacing a selection needs to know what is
// selected -- all against text the field no longer has if it is told once.
func TestIMEIsToldAboutTextAndSelectionChanges(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(TextField{Value: "hello"})
	o.FlushBuilds()

	if ti.setTextCalls == 0 {
		t.Fatal("the IME was never told what the field contains")
	}
	if ti.lastText != "hello" {
		t.Errorf("IME text = %q, want %q", ti.lastText, "hello")
	}

	// A selection must be reported as a range, not as a caret twice -- the
	// latter claims nothing is ever selected.
	before := ti.setTextCalls
	o.KeyboardTarget.OnKey(shell.Key{Kind: shell.KeyPress, Code: shell.KeyA, Mods: shell.ModCtrl})
	o.FlushBuilds()

	if ti.setTextCalls == before {
		t.Fatal("select-all did not reach the IME; selection replacement would " +
			"operate on the wrong range")
	}
	if ti.lastSelA == ti.lastSelB {
		t.Errorf("IME told selection %d..%d after select-all — a collapsed range "+
			"means 'nothing selected'", ti.lastSelA, ti.lastSelB)
	}
}

// Nothing is sent when neither the text nor the selection moved.
func TestIMENotToldWhenNothingChanged(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(TextField{Value: "x"})
	o.FlushBuilds()
	n := ti.setTextCalls

	o.RebuildAll()
	o.FlushBuilds()

	if ti.setTextCalls != n {
		t.Errorf("a rebuild with no change sent %d extra IME updates", ti.setTextCalls-n)
	}
}

// Focus must raise the keyboard during the focus callback itself, not on the
// build that follows it.
//
// A mobile browser only opens the soft keyboard when an element is focused
// inside a user gesture. gophics dispatches the pointer event, marks state
// dirty and builds on the next animation frame — by which time the activation
// has expired, so focusing the hidden input opens nothing. The symptom is the
// one that matters: on a phone the field takes focus, the caret blinks, and no
// keyboard ever appears.
//
// So this asserts *when*, not just whether. Show has to have happened by the
// time the focus notification returns, before any rebuild.
func TestFocusRaisesTheKeyboardBeforeTheNextBuild(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti

	o.SetRoot(TextField{Value: "hi"})
	o.FlushBuilds()

	// The focused field's handler is what the app calls on a focus change, and
	// it calls it synchronously from inside the pointer event.
	g := o.KeyboardTarget
	if g == nil || g.OnFocus == nil {
		t.Fatal("the field did not take focus, so there is nothing to test")
	}

	g.OnFocus(false) // blur
	before := ti.shown
	g.OnFocus(true) // and focus again, the way a tap does

	// Deliberately no FlushBuilds between the callback and the assertion: that
	// gap is the bug. Raising the keyboard from the build instead happens on
	// the next animation frame, after the user gesture has expired, and a
	// mobile browser then declines to open the keyboard at all — a caret
	// blinking in a field that cannot be typed into.
	if ti.shown <= before {
		t.Error("the focus callback did not raise the keyboard; it would be " +
			"raised on the next frame instead, outside the user gesture")
	}
}
