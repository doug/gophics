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
