package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// TestOnPressEnd verifies the press-end signal that drives touch press feedback:
// it fires exactly once per press that reached OnPress — whether the press ends
// in a tap or is cancelled by a slop-crossing drag — and it must not fire a tap
// when cancelled.
func TestOnPressEnd(t *testing.T) {
	var press, end, tap int
	root := widget.Interactive{
		Gestures: widget.Gestures{
			OnPress:    func(geom.Pt) { press++ },
			OnPressEnd: func() { end++ },
			OnTap:      func() { tap++ },
		},
		Child: widget.Canvas{Draw: func(paint.Canvas, geom.Size) {}}, // fills the window
	}
	h, err := NewHeadless(root, Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.layoutForInput()
	at := func(k shell.PointerKind, x, y float32) {
		h.core.Pointer(shell.Pointer{Kind: k, Pos: geom.Pt{X: x, Y: y}})
	}

	// A clean tap: press, tap, and press-end each fire once.
	at(shell.PointerDown, 100, 100)
	at(shell.PointerUp, 100, 100)
	if press != 1 || tap != 1 || end != 1 {
		t.Fatalf("clean tap: press=%d tap=%d end=%d, want 1/1/1", press, tap, end)
	}

	// Press then drag away past the slop: the press is cancelled — press-end
	// fires (clearing the highlight), but no tap.
	press, tap, end = 0, 0, 0
	at(shell.PointerDown, 100, 100)
	at(shell.PointerMove, 100, 160) // 60px > tapSlop
	at(shell.PointerUp, 100, 160)
	if press != 1 || tap != 0 || end != 1 {
		t.Fatalf("drag-away: press=%d tap=%d end=%d, want press=1 tap=0 end=1", press, tap, end)
	}
}
