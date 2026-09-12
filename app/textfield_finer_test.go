package app

import (
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Up, Up, Down comes back to the column you left, even across a short line —
// the goal column every native field keeps and this one did not.
func TestVerticalMovementKeepsGoalColumn(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "long first line\nab\nlong first line", Multiline: true}, pcKeys)
	// nativeField leaves the caret at the very end; go to the end of the
	// first line's "first" so the column is clearly past the short line.
	key(h, shell.KeyHome, shell.ModCtrl) // document start (PC)
	for i := 0; i < len("long first"); i++ {
		key(h, shell.KeyRight, 0)
	}
	key(h, shell.KeyDown, 0) // onto "ab": clamps to its end
	key(h, shell.KeyDown, 0) // onto the third line: back to the same x
	h.Type("X")
	if !strings.HasSuffix(st.value, "\nlong firstX line") {
		t.Errorf("after Down, Down the caret lost its column: %q", st.value)
	}
}

// Up on the first line goes to the start of the text; Down on the last line
// to the end — instead of doing nothing.
func TestVerticalMovementPastTheEndsGoesToTheEnds(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "one\ntwo", Multiline: true}, pcKeys)
	key(h, shell.KeyUp, 0) // from the end of "two" to "one"
	key(h, shell.KeyUp, 0) // past the first line: document start
	h.Type("X")
	if !strings.HasPrefix(st.value, "X") {
		t.Errorf("Up past the first line should reach the start: %q", st.value)
	}
	key(h, shell.KeyDown, 0)
	key(h, shell.KeyDown, 0)
	h.Type("Y")
	if !strings.HasSuffix(st.value, "Y") {
		t.Errorf("Down past the last line should reach the end: %q", st.value)
	}
}

// Ctrl+Right on a PC lands at the start of the next word; Alt+Right on a Mac
// at the end of the current one.
func TestWordRightConventionDiffers(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "one two"}, pcKeys)
	key(h, shell.KeyHome, 0)
	key(h, shell.KeyRight, shell.ModCtrl)
	h.Type("X")
	if st.value != "one Xtwo" {
		t.Errorf("pc Ctrl+Right: %q, want the start of 'two'", st.value)
	}
	h, st = nativeField(t, widget.TextField{Value: "one two"}, macKeys)
	key(h, shell.KeyLeft, shell.ModSuper) // line start
	key(h, shell.KeyRight, shell.ModAlt)
	h.Type("X")
	if st.value != "oneX two" {
		t.Errorf("mac Alt+Right: %q, want the end of 'one'", st.value)
	}
}

// Home in a wrapped field is the start of the visual line, not the paragraph.
func TestHomeInWrappedTextIsTheVisualLine(t *testing.T) {
	// 400px wide field minus padding wraps this comfortably past one line.
	long := strings.Repeat("word ", 30)
	h, st := nativeField(t, widget.TextField{Value: long, Multiline: true}, pcKeys)
	key(h, shell.KeyHome, 0) // start of the last visual line, not index 0
	h.Type("X")
	if strings.HasPrefix(st.value, "X") {
		t.Error("Home went to the paragraph start; a wrapped field's Home is the visual line")
	}
	if !strings.Contains(st.value, "Xword") {
		t.Errorf("Home did not land at a wrapped line's start: …%q", st.value[len(st.value)-40:])
	}
}

// Double-click then drag selects by whole words: the drag never leaves a
// partial word at either end.
func TestDoubleClickDragSelectsByWords(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "alpha beta gamma delta"}, pcKeys)
	// Click on "alpha", then press again and drag into "gamma" without
	// releasing — the native gesture.
	h.Press(geom.Pt{X: 20, Y: 30})
	h.Release(geom.Pt{X: 20, Y: 30})
	h.Step(0.05)
	h.Press(geom.Pt{X: 20, Y: 30})
	h.Move(geom.Pt{X: 120, Y: 30})
	h.Render()
	key(h, shell.KeyC, shell.ModCtrl)
	got := h.Clipboard().S
	if !strings.HasPrefix(got, "alpha") || !strings.HasSuffix(got, "gamma") {
		t.Errorf("word-drag selected %q, want whole words from alpha through gamma", got)
	}
}

// A finger tapping inside its own selection keeps it and opens the menu; a
// mouse click there collapses it.
func TestTouchTapInsideSelectionKeepsIt(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "alpha beta"}, pcKeys)
	key(h, shell.KeyA, shell.ModCtrl) // select all
	h.TouchPress(geom.Pt{X: 40, Y: 30})
	h.TouchRelease(geom.Pt{X: 40, Y: 30})
	h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05)
	h.Render()
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "alpha beta" {
		t.Errorf("a touch inside the selection collapsed it; copied %q", got)
	}
	var labels []string
	for _, n := range h.Semantics() {
		labels = append(labels, n.Label)
	}
	if !strings.Contains(strings.Join(labels, "|"), "Copy") {
		t.Error("a touch inside the selection did not open the edit menu")
	}

	h, _ = nativeField(t, widget.TextField{Value: "alpha beta"}, pcKeys)
	key(h, shell.KeyA, shell.ModCtrl)
	h.Tap(geom.Pt{X: 40, Y: 30})
	h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05)
	h.Render()
	h.Clipboard().S = ""
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "" {
		t.Errorf("a mouse click inside the selection kept it; copied %q", got)
	}
}

// A password field on touch shows the character just typed, and hides it as
// soon as the caret moves.
func TestPasswordRevealsLastTypedCharOnTouch(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "ab", Obscure: true}, pcKeys)
	h.TouchPress(geom.Pt{X: 380, Y: 30}) // a finger, so the reveal applies
	h.TouchRelease(geom.Pt{X: 380, Y: 30})
	h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05)
	h.Type("z")
	h.Render()
	shown := func() string {
		for _, n := range h.Semantics() {
			if n.Role == 0 || n.Value != "" {
				return n.Value
			}
		}
		return ""
	}
	if got := shown(); !strings.HasSuffix(got, "z") || strings.Contains(got, "a") {
		t.Errorf("after typing on touch the field shows %q, want bullets then a visible z", got)
	}
	key(h, shell.KeyLeft, 0)
	if got := shown(); strings.Contains(got, "z") {
		t.Errorf("moving the caret should hide the revealed character: %q", got)
	}
}
