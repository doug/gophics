package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// keyOnly is focusable but wants no keyboard: it handles Enter, like a button.
type keyOnly struct{}

func (keyOnly) Build(widget.Ctx) widget.Widget {
	return widget.Interactive{
		Gestures: widget.Gestures{OnKey: func(shell.Key) {}},
		Child:    widget.Sized{W: 10, H: 10},
	}
}

func handlerFor(t *testing.T, root widget.Widget) *shellHandler {
	t.Helper()
	h, err := NewHeadless(root, Config{Size: geom.Size{W: 100, H: 100}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return &shellHandler{core: h.core}
}

// An embedded host polls TextInputActive and raises the keyboard when it turns
// true. It must therefore mean "something accepts typed text", not "something
// has focus" -- a widget becomes focusable by handling OnText or OnKey, and one
// focusable widget is usually mounted from the first frame, since a focusable
// widget mounted while nothing has focus takes it. Reporting focus meant the
// answer was already true before any field existed, the host saw no transition,
// and the keyboard never appeared.
func TestTextInputActiveOnlyForTextAcceptingFocus(t *testing.T) {
	if got := handlerFor(t, keyOnly{}).TextInputActive(); got {
		t.Error("a key-only widget (a button handling Enter) reported wanting the " +
			"on-screen keyboard; the host would see no transition when a real " +
			"field is focused later")
	}

	if got := handlerFor(t, widget.TextField{}).TextInputActive(); !got {
		t.Error("a focused text field did not report wanting the keyboard — " +
			"there would be no way to type on a phone")
	}
}

// Nothing focusable at all must report false rather than panic.
func TestTextInputActiveWithNothingFocused(t *testing.T) {
	if handlerFor(t, widget.Sized{W: 10, H: 10}).TextInputActive() {
		t.Error("nothing focusable, but the keyboard was requested")
	}
}
