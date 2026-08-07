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

// TestPenStrokeCommits: with the default pen tool, a drag inside the canvas
// commits one freehand element carrying the dragged points.
func TestPenStrokeCommits(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.elements)

	h.Drag(geom.Pt{X: 120, Y: 200}, geom.Pt{X: 320, Y: 300}) // down→move→up
	if len(g.elements) != base+1 {
		t.Fatalf("stroke not committed: have %d, want %d", len(g.elements), base+1)
	}
	last := g.elements[len(g.elements)-1]
	if last.tool != toolPen || len(last.pts) < 2 {
		t.Fatalf("committed element wrong: tool=%d pts=%d", last.tool, len(last.pts))
	}
}

// TestTapMakesDot: a tap (no drag) still commits a one-point pen element (a dot).
func TestTapMakesDot(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.elements)
	h.Tap(geom.Pt{X: 500, Y: 300})
	if len(g.elements) != base+1 {
		t.Fatalf("tap didn't commit a dot: have %d, want %d", len(g.elements), base+1)
	}
	if last := g.elements[len(g.elements)-1]; last.tool != toolPen || len(last.pts) != 1 {
		t.Fatalf("dot wrong: tool=%d pts=%d", last.tool, len(last.pts))
	}
}

// TestShapeCommits: with a shape tool selected, a drag commits one shape element
// whose two corners are the gesture endpoints.
func TestShapeCommits(t *testing.T) {
	g, h := newBoard(t)
	h.Tap(center(g.toolBtn[toolRect]))
	if g.tool != toolRect {
		t.Fatalf("rect tool not selected: %d", g.tool)
	}
	base := len(g.elements)
	h.Drag(geom.Pt{X: 150, Y: 200}, geom.Pt{X: 300, Y: 320})
	if len(g.elements) != base+1 {
		t.Fatalf("shape not committed: have %d, want %d", len(g.elements), base+1)
	}
	last := g.elements[len(g.elements)-1]
	if last.tool != toolRect || bounds(last.a, last.b).IsEmpty() {
		t.Fatalf("shape element wrong: tool=%d bounds=%v", last.tool, bounds(last.a, last.b))
	}
}

// TestUndoRedo: drawing then Undo restores the prior set; Redo reapplies it.
func TestUndoRedo(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.elements)

	h.Drag(geom.Pt{X: 120, Y: 200}, geom.Pt{X: 320, Y: 300})
	if len(g.elements) != base+1 {
		t.Fatalf("draw failed: %d", len(g.elements))
	}
	h.Tap(center(g.undoBtn))
	if len(g.elements) != base {
		t.Fatalf("undo didn't restore: have %d, want %d", len(g.elements), base)
	}
	h.Tap(center(g.redoBtn))
	if len(g.elements) != base+1 {
		t.Fatalf("redo didn't reapply: have %d, want %d", len(g.elements), base+1)
	}
}

// TestClearAndUndo: Clear empties the board; Undo brings it back.
func TestClearAndUndo(t *testing.T) {
	g, h := newBoard(t)
	base := len(g.elements)
	if base == 0 {
		t.Fatal("expected sample elements")
	}
	h.Tap(center(g.clearBtn))
	if len(g.elements) != 0 {
		t.Fatalf("clear left %d elements", len(g.elements))
	}
	h.Tap(center(g.undoBtn))
	if len(g.elements) != base {
		t.Fatalf("undo after clear restored %d, want %d", len(g.elements), base)
	}
}

// TestToolSelection: swatches, tool buttons, and width buttons update the board.
func TestToolSelection(t *testing.T) {
	g, h := newBoard(t)

	h.Tap(center(g.swatch[1])) // red
	if g.col != palette[1] {
		t.Fatalf("color select failed: col=%v", g.col)
	}
	h.Tap(center(g.toolBtn[toolEllipse]))
	if g.tool != toolEllipse {
		t.Fatalf("tool select failed: tool=%d", g.tool)
	}
	h.Tap(center(g.widthBtn[2])) // thickest
	if g.w != widths[2] {
		t.Fatalf("width select failed: w=%v want %v", g.w, widths[2])
	}
}

// TestEraserRemovesElement: with the eraser tool, tapping on an element removes
// it, and Undo brings it back.
func TestEraserRemovesElement(t *testing.T) {
	g, h := newBoard(t)
	h.Tap(center(g.clearBtn)) // start from an empty board

	h.Drag(geom.Pt{X: 200, Y: 250}, geom.Pt{X: 300, Y: 250}) // one pen stroke
	if len(g.elements) != 1 {
		t.Fatalf("setup draw failed: %d", len(g.elements))
	}
	h.Tap(center(g.toolBtn[toolEraser]))
	h.Tap(geom.Pt{X: 250, Y: 250}) // on the stroke
	if len(g.elements) != 0 {
		t.Fatalf("eraser didn't remove element: %d left", len(g.elements))
	}
	h.Tap(center(g.undoBtn))
	if len(g.elements) != 1 {
		t.Fatalf("undo after erase restored %d, want 1", len(g.elements))
	}
}

// TestStrokesStayInCanvas: dragging up into the toolbar band doesn't add points
// above the canvas.
func TestStrokesStayInCanvas(t *testing.T) {
	g, h := newBoard(t)
	// Start in the canvas, drag up into the toolbar (y < toolbarH): points
	// outside the draw area are dropped.
	h.DragTo(geom.Pt{X: 400, Y: 300}, geom.Pt{X: 400, Y: 20})
	h.Release(geom.Pt{X: 400, Y: 20})
	last := g.elements[len(g.elements)-1]
	for _, p := range last.pts {
		if p.Y < toolbarH {
			t.Fatalf("stroke drew into the toolbar band: point %v", p)
		}
	}
}
