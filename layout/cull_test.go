package layout

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
)

// paintCounter is a fixed-size test box that counts how many times it painted.
type paintCounter struct {
	Base
	w, h   float32
	paints int
}

func (l *paintCounter) Layout(cs Constraints) geom.Size {
	return l.Done(cs, cs.Constrain(geom.Size{W: l.w, H: l.h}))
}
func (l *paintCounter) Paint(c paint.Canvas, at geom.Pt) { l.paints++ }
func (l *paintCounter) AddHits(p geom.Pt, hits *[]Hit)   {}

// TestFlexViewportCulling verifies that a scrolled Column paints only its
// on-screen rows (viewport culling) while still painting every row that is even
// partially visible — i.e. culling never drops visible content.
func TestFlexViewportCulling(t *testing.T) {
	const (
		rowH   = 50
		nRows  = 20
		viewH  = 100
		offset = 310 // rows straddle both viewport edges at this offset
	)
	rows := make([]Box, nRows)
	counters := make([]*paintCounter, nRows)
	for i := range counters {
		counters[i] = &paintCounter{w: 100, h: rowH}
		rows[i] = counters[i]
	}
	vp := &Viewport{Axis: Vertical, Child: Column(rows...)}
	vp.Layout(Tight(sz(100, viewH)))
	vp.Offset = offset

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	// Screen y of row i is rowH*i - offset; it's visible if that span overlaps
	// [0, viewH). At offset 310 that is rows 6, 7, 8 (6 and 8 straddle the edges).
	for i, c := range counters {
		top := float32(rowH*i) - offset
		visible := top < viewH && top+rowH > 0
		switch {
		case visible && c.paints == 0:
			t.Errorf("row %d is visible (screen y %.0f) but was culled — dropped visible content", i, top)
		case !visible && c.paints != 0:
			t.Errorf("row %d is off-screen (screen y %.0f) but painted %d times — not culled", i, top, c.paints)
		}
	}

	// Sanity: far more rows are off-screen than on, so most must be culled.
	painted := 0
	for _, c := range counters {
		if c.paints > 0 {
			painted++
		}
	}
	if painted != 3 {
		t.Errorf("painted %d rows, want 3 (rows 6–8); culling window is wrong", painted)
	}
}

// TestTranslatedInkNotCulled verifies culling tests ink bounds, not layout
// rects: a Translated child whose layout rect is outside the viewport but
// whose ink is translated into view must still paint — and a plain offscreen
// sibling must still be culled.
func TestTranslatedInkNotCulled(t *testing.T) {
	inked := &paintCounter{w: 100, h: 50}
	plain := &paintCounter{w: 100, h: 50}
	vp := &Viewport{Axis: Vertical, Child: Column(
		&paintCounter{w: 100, h: 100}, // filler occupying the viewport
		// Layout rect y=[100,150) — offscreen; ink shifted to y=[25,75) — visible.
		&Translated{Dy: -75, Child: inked},
		plain, // layout rect y=[150,200) — offscreen, no ink shift
	)}
	vp.Layout(Tight(sz(100, 100)))

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	if inked.paints == 0 {
		t.Error("Translated child with visible ink was culled — dropped visible content")
	}
	if plain.paints != 0 {
		t.Errorf("plain offscreen sibling painted %d times — not culled", plain.paints)
	}
}

// TestNestedTranslatedTransformedInkNotCulled verifies ink bounds compose:
// Translated(Transformed(x)) whose combined mapping brings offscreen layout
// into view must not be culled.
func TestNestedTranslatedTransformedInkNotCulled(t *testing.T) {
	inked := &paintCounter{w: 100, h: 25}
	vp := &Viewport{Axis: Vertical, Child: Column(
		&paintCounter{w: 100, h: 150}, // pushes the next row below the fold
		// Layout rect y=[150,175). Transformed scales the 25-tall child to 50
		// about its top-left; Translated then lifts it by 125: ink y=[25,75).
		&Translated{Dy: -125, Child: &Transformed{
			T:     paint.Transform{SY: 2},
			Child: inked,
		}},
	)}
	vp.Layout(Tight(sz(100, 100)))

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	if inked.paints == 0 {
		t.Error("Translated(Transformed(x)) with visible ink was culled — nested ink bounds broken")
	}
}

// TestGridCulling verifies a mostly-offscreen Grid paints only its visible
// cells, and every partially-visible cell.
func TestGridCulling(t *testing.T) {
	const (
		cellH  = 50
		n      = 20
		viewH  = 100
		offset = 100
	)
	cells := make([]Box, n)
	counters := make([]*paintCounter, n)
	for i := range counters {
		counters[i] = &paintCounter{w: 50, h: cellH}
		cells[i] = counters[i]
	}
	vp := &Viewport{Axis: Vertical, Child: &Grid{Columns: 2, Children: cells}}
	vp.Layout(Tight(sz(100, viewH)))
	vp.Offset = offset

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	for i, c := range counters {
		top := float32(cellH*(i/2)) - offset
		visible := top < viewH && top+cellH > 0
		switch {
		case visible && c.paints == 0:
			t.Errorf("grid cell %d is visible (screen y %.0f) but was culled", i, top)
		case !visible && c.paints != 0:
			t.Errorf("grid cell %d is off-screen (screen y %.0f) but painted", i, top)
		}
	}
}

// TestWrapCulling verifies a mostly-offscreen Wrap paints only its visible
// children, and every partially-visible one.
func TestWrapCulling(t *testing.T) {
	const (
		childH = 50
		n      = 20
		viewH  = 100
		offset = 100
	)
	kids := make([]Box, n)
	counters := make([]*paintCounter, n)
	for i := range counters {
		counters[i] = &paintCounter{w: 50, h: childH}
		kids[i] = counters[i]
	}
	// Width 100 fits two 50-wide children per run: same geometry as the grid.
	vp := &Viewport{Axis: Vertical, Child: &Wrap{Children: kids}}
	vp.Layout(Tight(sz(100, viewH)))
	vp.Offset = offset

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	for i, c := range counters {
		top := float32(childH*(i/2)) - offset
		visible := top < viewH && top+childH > 0
		switch {
		case visible && c.paints == 0:
			t.Errorf("wrap child %d is visible (screen y %.0f) but was culled", i, top)
		case !visible && c.paints != 0:
			t.Errorf("wrap child %d is off-screen (screen y %.0f) but painted", i, top)
		}
	}
}

// TestStackCulling verifies both halves of Stack culling: a Stack whose
// layout rect is fully below the fold is kept by its parent because a child's
// ink reaches into view (Stack.InkBounds), and Stack's own paint culls only
// the genuinely offscreen child.
func TestStackCulling(t *testing.T) {
	offscreen := &paintCounter{w: 100, h: 50}
	visible := &paintCounter{w: 100, h: 50}
	vp := &Viewport{Axis: Vertical, Child: Column(
		&paintCounter{w: 100, h: 200}, // pushes the Stack below the fold
		&Stack{Children: []Box{
			offscreen, // ink y=[200,250) — offscreen
			// Ink lifted to y=[50,100) — visible.
			&Translated{Dy: -150, Child: visible},
		}},
	)}
	vp.Layout(Tight(sz(100, 100)))

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	if visible.paints == 0 {
		t.Error("Stack child with visible ink was culled — Stack ink bounds or child cull broken")
	}
	if offscreen.paints != 0 {
		t.Errorf("offscreen Stack child painted %d times — not culled", offscreen.paints)
	}
}

// TestUnboundedInkNeverCulled pins the opt-out: geom.Unbounded ink (what an
// unclipped widget.Canvas reports) always overlaps any clip, so such a box
// is never culled no matter where it sits.
func TestUnboundedInkNeverCulled(t *testing.T) {
	c := &unboundedInkBox{}
	vp := &Viewport{Axis: Vertical, Child: Column(
		&paintCounter{w: 100, h: 500}, // pushes c far below the fold
		c,
	)}
	vp.Layout(Tight(sz(100, 100)))

	var list scene.List
	vp.Paint(list.Recorder(), geom.Pt{})

	if c.paints == 0 {
		t.Error("box with Unbounded ink was culled — unclipped-Canvas semantics broken")
	}
}

// unboundedInkBox mimics an unclipped widget.Canvas: it reports Unbounded ink.
type unboundedInkBox struct {
	paintCounter
}

func (b *unboundedInkBox) InkBounds() geom.Rect { return geom.Unbounded }
