package app

import (
	"image/color"
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// selected copies the selection and returns it: the one readback of the
// selection a test has without reaching into the widget.
func selected(h *Headless, cmd shell.Mods) string {
	h.Clipboard().S = ""
	key(h, shell.KeyC, cmd)
	return h.Clipboard().S
}

// A drag selection held past the field's edge keeps scrolling: the pointer
// cannot reach text that is out of view, so the field brings it into view
// for as long as the pointer stays there, and stops when it lifts.
func TestDragSelectionAutoScrollsPastTheEdge(t *testing.T) {
	long := strings.Repeat("word ", 40)
	h, _ := nativeField(t, widget.TextField{Value: long}, pcKeys)
	key(h, shell.KeyHome, 0) // scrolled to the start
	h.Press(geom.Pt{X: 40, Y: 30})
	h.Move(geom.Pt{X: 396, Y: 30}) // past the right edge of the box
	h.Render()
	first := len(selected(h, shell.ModCtrl))
	if first == 0 {
		t.Fatal("dragging to the edge selected nothing")
	}
	for i := 0; i < 10; i++ {
		h.Step(0.05)
	}
	h.Render()
	held := len(selected(h, shell.ModCtrl))
	if held <= first {
		t.Fatalf("holding the pointer past the edge did not extend the selection: %d then %d", first, held)
	}
	h.Release(geom.Pt{X: 396, Y: 30})
	for i := 0; i < 10; i++ {
		h.Step(0.05)
	}
	h.Render()
	if after := len(selected(h, shell.ModCtrl)); after != held {
		t.Errorf("the selection kept growing after the pointer lifted: %d then %d", held, after)
	}
	if len(selected(h, shell.ModCtrl)) >= len([]rune(long)) {
		t.Errorf("a short hold reached the very end (%d runes); the scroll is too fast to be a scroll", len(long))
	}
}

// The same past the bottom of a wrapped field: one line further per step.
func TestDragSelectionAutoScrollsPastTheBottom(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "one\ntwo\nthree\nfour\nfive\nsix\nseven", Multiline: true}, pcKeys)
	key(h, shell.KeyHome, shell.ModCtrl)
	h.Press(geom.Pt{X: 12, Y: 12})
	h.Move(geom.Pt{X: 12, Y: 59}) // below the box, which the window clips
	h.Render()
	first := len(selected(h, shell.ModCtrl))
	for i := 0; i < 6; i++ {
		h.Step(0.05)
	}
	h.Render()
	if held := len(selected(h, shell.ModCtrl)); held <= first {
		t.Fatalf("holding below the box did not select further lines: %d then %d", first, held)
	}
	h.Release(geom.Pt{X: 12, Y: 59})
}

// clearMods ends the modifier state the last key event left behind, so a
// press that follows a shift-selection is not a shift-click.
func clearMods(h *Headless) { h.KeyMod(shell.KeyShift, 0) }

// A mouse press inside the selection picks the text up rather than
// collapsing it; a drag carries it, and the release drops it — moved, or
// copied with Alt — as one undo step. A press that never moves places the
// caret on release, which is when Cocoa and Windows place it too.
func TestDragSelectedTextMovesIt(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "alpha beta gamma"}, macKeys)
	key(h, shell.KeyLeft, shell.ModSuper)               // line start
	key(h, shell.KeyRight, shell.ModAlt|shell.ModShift) // select "alpha"
	if got := selected(h, shell.ModSuper); got != "alpha" {
		t.Fatalf("setup selected %q", got)
	}
	clearMods(h)
	h.Press(geom.Pt{X: 22, Y: 30}) // inside "alpha"
	h.Move(geom.Pt{X: 200, Y: 30})
	h.Move(geom.Pt{X: 380, Y: 30}) // past the end of the text
	h.Render()
	if got := selected(h, shell.ModSuper); got != "alpha" {
		t.Errorf("carrying the text changed the selection to %q", got)
	}
	h.Release(geom.Pt{X: 380, Y: 30})
	h.Render()
	if st.value != " beta gammaalpha" {
		t.Fatalf("moved value = %q", st.value)
	}
	if got := selected(h, shell.ModSuper); got != "alpha" {
		t.Errorf("the dropped text should stay selected, got %q", got)
	}
	key(h, shell.KeyZ, shell.ModSuper)
	if st.value != "alpha beta gamma" {
		t.Errorf("one undo should put the text back, got %q", st.value)
	}

	// Alt held at the drop copies instead.
	h, st = nativeField(t, widget.TextField{Value: "alpha beta gamma"}, macKeys)
	key(h, shell.KeyLeft, shell.ModSuper)
	key(h, shell.KeyRight, shell.ModAlt|shell.ModShift)
	clearMods(h)
	h.Press(geom.Pt{X: 22, Y: 30})
	h.Move(geom.Pt{X: 380, Y: 30})
	h.KeyMod(shell.KeyShift, shell.ModAlt) // Alt down: the modifier state the release sees
	h.Release(geom.Pt{X: 380, Y: 30})
	h.Render()
	if st.value != "alpha beta gammaalpha" {
		t.Errorf("copied value = %q", st.value)
	}

	// A click inside the selection that never moves collapses to the caret.
	h, _ = nativeField(t, widget.TextField{Value: "alpha beta gamma"}, macKeys)
	key(h, shell.KeyLeft, shell.ModSuper)
	key(h, shell.KeyRight, shell.ModAlt|shell.ModShift)
	clearMods(h)
	h.Press(geom.Pt{X: 22, Y: 30})
	h.Render()
	if got := selected(h, shell.ModSuper); got != "alpha" {
		t.Errorf("the press alone should not collapse the selection, got %q", got)
	}
	h.Release(geom.Pt{X: 22, Y: 30})
	h.Render()
	if got := selected(h, shell.ModSuper); got != "" {
		t.Errorf("a click inside the selection should collapse it, got %q", got)
	}
}

// A finger dragging the caret gets the magnifier above it; it goes away
// when the finger lifts. A mouse never sees it.
func TestFingerDragShowsMagnifier(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "alpha beta gamma delta"}, pcKeys)
	// A spot the loupe covers and nothing else paints: the window is
	// 400x60, the field starts at y=10 and its text ends well before
	// x=250, and the loupe, 128 wide and centred on a finger at x=200,
	// wants to sit 16px above y=30 and is pushed down to y=8.
	probe := func() color.Color { return h.Render().At(250, 13) }
	lum := func(c color.Color) uint32 { r, g, b, _ := c.RGBA(); return (r + g + b) / 3 }
	before := lum(probe())

	h.Press(geom.Pt{X: 380, Y: 30})
	h.Move(geom.Pt{X: 200, Y: 30})
	if l := lum(probe()); l != before {
		t.Errorf("a mouse drag painted something over the field (%d vs %d)", l, before)
	}
	h.Release(geom.Pt{X: 200, Y: 30})

	h.TouchPress(geom.Pt{X: 380, Y: 30})
	h.TouchMove(geom.Pt{X: 200, Y: 30})
	if l := lum(probe()); l == before {
		t.Fatal("a finger drag did not raise the magnifier")
	}
	h.TouchRelease(geom.Pt{X: 200, Y: 30})
	if l := lum(probe()); l != before {
		t.Errorf("the magnifier stayed after the finger lifted (%d vs %d)", l, before)
	}
}
