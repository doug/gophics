package main

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

func newBoard(t *testing.T) (*board, *app.Headless) {
	t.Helper()
	var g *board
	stateHook = func(gg *board) { g = gg }
	t.Cleanup(func() { stateHook = nil })
	h, err := app.NewHeadless(Board{}, app.Config{Size: geom.Size{W: 760, H: 460}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount + draw (sets drawArea + toolbar rects)
	if g == nil {
		t.Fatal("Init hook never fired")
	}
	return g, h
}

func center(r geom.Rect) geom.Pt {
	return geom.Pt{X: r.Min.X + r.Dx()/2, Y: r.Min.Y + r.Dy()/2}
}

// TestDrawStrokeCommits: a drag inside the canvas commits one new stroke whose
// path carries the dragged points.
func TestDrawStrokeCommits(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.strokes)

	h.Drag(geom.Pt{X: 120, Y: 200}, geom.Pt{X: 320, Y: 300}) // down→move→up
	if len(g.strokes) != base+1 {
		t.Fatalf("stroke not committed: have %d, want %d", len(g.strokes), base+1)
	}
	last := g.strokes[len(g.strokes)-1]
	if last.n < 2 || last.path.Empty() {
		t.Fatalf("committed stroke has no geometry: n=%d empty=%v", last.n, last.path.Empty())
	}
}

// TestTapMakesDot: a tap (no drag) still commits a stroke (a round dot).
func TestTapMakesDot(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.strokes)
	h.Tap(geom.Pt{X: 500, Y: 300})
	if len(g.strokes) != base+1 {
		t.Fatalf("tap didn't commit a dot: have %d, want %d", len(g.strokes), base+1)
	}
}

// TestUndoRedo: drawing then Undo restores the prior set; Redo reapplies it.
func TestUndoRedo(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.strokes)

	h.Drag(geom.Pt{X: 120, Y: 200}, geom.Pt{X: 320, Y: 300})
	if len(g.strokes) != base+1 {
		t.Fatalf("draw failed: %d", len(g.strokes))
	}
	h.Tap(center(g.undoBtn))
	if len(g.strokes) != base {
		t.Fatalf("undo didn't restore: have %d, want %d", len(g.strokes), base)
	}
	h.Tap(center(g.redoBtn))
	if len(g.strokes) != base+1 {
		t.Fatalf("redo didn't reapply: have %d, want %d", len(g.strokes), base+1)
	}
}

// TestClearAndUndo: Clear empties the board; Undo brings it back.
func TestClearAndUndo(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.strokes)
	if base == 0 {
		t.Fatal("expected sample strokes")
	}
	h.Tap(center(g.clearBtn))
	if len(g.strokes) != 0 {
		t.Fatalf("clear left %d strokes", len(g.strokes))
	}
	h.Tap(center(g.undoBtn))
	if len(g.strokes) != base {
		t.Fatalf("undo after clear restored %d, want %d", len(g.strokes), base)
	}
}

// TestToolSelection: swatches, eraser, and width buttons update the pen.
func TestToolSelection(t *testing.T) {
	g, h := newBoard(t)

	h.Tap(center(g.swatch[1])) // red
	if g.col != palette[1] || g.eraser {
		t.Fatalf("color select failed: col=%v eraser=%v", g.col, g.eraser)
	}
	h.Tap(center(g.eraserBtn))
	if !g.eraser {
		t.Fatal("eraser not selected")
	}
	h.Tap(center(g.widthBtn[2])) // thickest
	if g.w != widths[2] {
		t.Fatalf("width select failed: w=%v want %v", g.w, widths[2])
	}
	// A stroke drawn with the eraser is paper-colored at the eraser width.
	h.Drag(geom.Pt{X: 200, Y: 250}, geom.Pt{X: 260, Y: 250})
	last := g.strokes[len(g.strokes)-1]
	if last.col != paper || last.w != eraseW {
		t.Fatalf("eraser stroke style wrong: col=%v w=%v", last.col, last.w)
	}
}

// TestStrokesStayInCanvas: dragging up into the toolbar band doesn't add points
// above the canvas.
func TestStrokesStayInCanvas(t *testing.T) {
	g, h := newBoard(t)
	// Start in the canvas, drag up into the toolbar (y < toolbarH): the point
	// outside the draw area is dropped.
	h.DragTo(geom.Pt{X: 400, Y: 300}, geom.Pt{X: 400, Y: 20})
	h.Release(geom.Pt{X: 400, Y: 20})
	last := g.strokes[len(g.strokes)-1]
	b := last.path.Bounds()
	if b.Min.Y < toolbarH {
		t.Fatalf("stroke drew into the toolbar band: bounds %v", b)
	}
}
