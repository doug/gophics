package widget

import (
	"testing"

	"github.com/doug/gophics/paint"
)

func TestTextFieldResolvedColors(t *testing.T) {
	// Unset caret defaults to the text color, so a field always shows a cursor.
	f := TextField{TextColor: paint.RGB(0.1, 0.2, 0.3)}
	txt, caret, sel, ph := f.resolvedColors()
	if caret != txt {
		t.Errorf("caret = %v, want it to default to text color %v", caret, txt)
	}
	if sel.A == 0 {
		t.Error("selection color should default to a visible tint")
	}
	if ph.A == 0 {
		t.Error("placeholder color should default to visible")
	}

	// Unset text color defaults to opaque, so the caret is visible even with no
	// colors configured at all.
	_, caret2, _, _ := TextField{}.resolvedColors()
	if caret2.A == 0 {
		t.Error("caret must be visible when no colors are set")
	}

	// An explicit caret color wins.
	red := paint.RGB(1, 0, 0)
	if _, c, _, _ := (TextField{CaretColor: red}).resolvedColors(); c != red {
		t.Errorf("explicit caret color = %v, want %v", c, red)
	}
}

func TestCaretBlink(t *testing.T) {
	o := &Owner{}
	o.SetRoot(TextField{})
	o.FlushBuilds()
	st := o.root.state.(*textFieldState) // Init ran, so st.ctx is valid

	st.focused = false
	if st.caretVisible() {
		t.Error("caret should be hidden when unfocused")
	}

	st.focused = true
	st.activity() // fresh activity → solid
	if !st.caretVisible() {
		t.Error("caret should be solid right after activity")
	}

	st.blink = caretPeriod * 0.75 // past the on-half → off
	if st.caretVisible() {
		t.Error("caret should blink off mid-period")
	}
	st.blink = caretPeriod * 1.10 // wrapped back into the on-half
	if !st.caretVisible() {
		t.Error("caret should blink back on next cycle")
	}

	// Typing/moving keeps it solid.
	st.activity()
	if !st.caretVisible() {
		t.Error("activity should reset the caret to solid")
	}

	// A selection hides the caret.
	st.ed.SetText("hello")
	st.ed.SelectAll()
	if st.caretVisible() {
		t.Error("caret should be hidden while a selection is active")
	}

	// Reduce-motion → always solid (no blink).
	st.ed.MoveTo(0, false) // clear selection
	o.ReduceMotion = true
	st.blink = caretPeriod * 0.75 // would be off if blinking
	if !st.caretVisible() {
		t.Error("caret should stay solid under reduce-motion")
	}
}
