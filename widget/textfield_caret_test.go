package widget

import (
	"testing"

	"github.com/doug/gossamer/paint"
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
