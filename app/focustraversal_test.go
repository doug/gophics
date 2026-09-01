package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

type threeFields struct{}

func (threeFields) Build(widget.Ctx) widget.Widget {
	f := func() widget.Widget {
		return widget.Sized{W: 200, H: 40, Child: widget.TextField{Value: ""}}
	}
	return widget.Column(f(), widget.Sized{H: 10}, f(), widget.Sized{H: 10}, f())
}

func newThree(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(threeFields{}, Config{Size: geom.Size{W: 300, H: 400}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func tab(h *Headless, shift bool) {
	mods := shell.Mods(0)
	if shift {
		mods = shell.ModShift
	}
	h.core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: shell.KeyTab, Mods: mods})
	h.Render()
}

// Tab walks the fields in build order and wraps.
//
// Without it a form is unusable from the keyboard: the only way to reach the
// second field is to point at it, which on a laptop is the wrong hand and on a
// phone keyboard is not possible at all.
func TestTabWalksFieldsInOrderAndWraps(t *testing.T) {
	h := newThree(t)

	first := h.core.Owner.KeyboardTarget
	if first == nil {
		t.Fatal("no field took focus on mount")
	}

	tab(h, false)
	second := h.core.Owner.KeyboardTarget
	if second == first || second == nil {
		t.Fatal("Tab did not move focus off the first field")
	}

	tab(h, false)
	third := h.core.Owner.KeyboardTarget
	if third == second || third == first {
		t.Fatalf("Tab did not reach a third distinct field")
	}

	// Off the end, back to the start: a form is a cycle, not a line.
	tab(h, false)
	if h.core.Owner.KeyboardTarget != first {
		t.Error("Tab past the last field did not wrap to the first")
	}
}

// Shift-Tab goes the other way, and wraps the other way too.
func TestShiftTabGoesBackwards(t *testing.T) {
	h := newThree(t)
	first := h.core.Owner.KeyboardTarget

	tab(h, false)
	second := h.core.Owner.KeyboardTarget
	tab(h, true)
	if h.core.Owner.KeyboardTarget != first {
		t.Error("Shift-Tab did not return to the previous field")
	}
	_ = second

	// Backwards off the front wraps to the last.
	tab(h, true)
	back := h.core.Owner.KeyboardTarget
	if back == first || back == nil {
		t.Error("Shift-Tab from the first field did not wrap to the last")
	}
}

// Moving focus by Tab must raise and lower the keyboard the same way a tap
// does, or the field is reachable but not typable on a phone.
func TestTabFiresFocusCallbacks(t *testing.T) {
	h := newThree(t)
	first := h.core.Owner.KeyboardTarget
	if first == nil || first.OnFocus == nil {
		t.Fatal("no focusable field with a focus callback")
	}

	tab(h, false)
	second := h.core.Owner.KeyboardTarget
	if second == nil || second.OnFocus == nil {
		t.Fatal("Tab landed somewhere without a focus callback")
	}
	if second == first {
		t.Fatal("focus did not move")
	}
}

// A multiline field keeps Tab for indentation; traversal must not take it.
func TestMultilineFieldKeepsTab(t *testing.T) {
	h, err := NewHeadless(
		widget.Column(
			widget.Sized{W: 200, H: 60, Child: widget.TextField{Multiline: true}},
			widget.Sized{W: 200, H: 40, Child: widget.TextField{}},
		),
		Config{Size: geom.Size{W: 300, H: 400}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	before := h.core.Owner.KeyboardTarget
	tab(h, false)
	if h.core.Owner.KeyboardTarget != before {
		t.Error("Tab moved focus out of a multiline field, which needs it to indent")
	}
}
