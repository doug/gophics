package chart

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// x-axis labels are centred on their tick, so it is their width — not the tick
// spacing — that decides whether they collide. Long labels on a narrow chart
// overlap into an unreadable smear, which is what a date or a hostname axis
// looks like on a phone.
func TestVisibleXLabelsDropsOverlappingOnes(t *testing.T) {
	area := geom.RectXYWH(0, 0, 200, 100)
	ticks := []Tick{{Pos: 0}, {Pos: 0.25}, {Pos: 0.5}, {Pos: 0.75}, {Pos: 1}}

	// Each label 60px wide: at 200px across, five of them cannot all fit.
	wide := func(string) float32 { return 60 }
	show := visibleXLabels(ticks, area, roomy(area), Axis{}, wide)

	kept := 0
	for _, v := range show {
		if v {
			kept++
		}
	}
	if kept == len(ticks) {
		t.Fatalf("kept all %d labels; 5 x 60px cannot fit in 200px without overlapping", kept)
	}
	if kept == 0 {
		t.Fatal("dropped every label; the axis would be unreadable in the other direction")
	}

	// Whatever survived must not overlap.
	prevRight := float32(0)
	first := true
	for i, t2 := range ticks {
		if !show[i] {
			continue
		}
		left := area.Min.X + t2.Pos*area.Dx() - 30
		if !first && left < prevRight {
			t.Errorf("label %d starts at %.0f but the previous one ended at %.0f",
				i, left, prevRight)
		}
		prevRight = left + 60
		first = false
	}
	t.Logf("kept %d of %d labels", kept, len(ticks))
}

// Narrow labels must all survive: thinning is for when they genuinely collide.
func TestVisibleXLabelsKeepsAllWhenTheyFit(t *testing.T) {
	area := geom.RectXYWH(0, 0, 600, 100)
	ticks := []Tick{{Pos: 0}, {Pos: 0.25}, {Pos: 0.5}, {Pos: 0.75}, {Pos: 1}}
	narrow := func(string) float32 { return 20 }

	for i, v := range visibleXLabels(ticks, area, roomy(area), Axis{}, narrow) {
		if !v {
			t.Errorf("dropped label %d even though 5 x 20px fits easily in 600px", i)
		}
	}
}

// An axis must never come back with nothing on it, however cramped.
func TestVisibleXLabelsAlwaysKeepsTheFirst(t *testing.T) {
	area := geom.RectXYWH(0, 0, 40, 100)
	ticks := []Tick{{Pos: 0}, {Pos: 0.5}, {Pos: 1}}
	huge := func(string) float32 { return 500 }

	show := visibleXLabels(ticks, area, roomy(area), Axis{}, huge)
	if !show[0] {
		t.Error("no labels at all on a very narrow axis")
	}
}

// roomy is the widget box around a plot area, wide enough that the end-clamp in
// xLabelLeft does not move anything: these tests are about labels colliding
// with each other, not with the edge.
func roomy(area geom.Rect) geom.Rect {
	return geom.Rect{
		Min: geom.Pt{X: area.Min.X - 100, Y: area.Min.Y},
		Max: geom.Pt{X: area.Max.X + 100, Y: area.Max.Y},
	}
}

// An end label stays inside the chart's box.
//
// A tick at the far right sits on area.Max.X, so a centred label runs half its
// width past it — into a right margin of 14pt, narrower than any date. The
// overflow was clipped by the chart's own box and the label lost its tail:
// "Nov '25" rendered as "Nov '2", which reads as a rendering fault rather than
// a layout one.
func TestEndLabelsStayInsideTheChart(t *testing.T) {
	// A plot area inset from the widget by the usual margins.
	bounds := geom.RectXYWH(0, 0, 300, 200)
	area := geom.RectXYWH(30, 10, 256, 160) // right margin of 14

	const w = 44 // about the width of "Nov '25"
	if got := xLabelLeft(area.Max.X, w, bounds); got+w > bounds.Max.X {
		t.Errorf("last label spans to %v, past the chart's right edge %v", got+w, bounds.Max.X)
	}
	if got := xLabelLeft(area.Min.X, w, bounds); got < bounds.Min.X {
		t.Errorf("first label starts at %v, before the chart's left edge %v", got, bounds.Min.X)
	}
	// A label away from the ends is still centred on its tick.
	mid := area.Min.X + area.Dx()/2
	if got, want := xLabelLeft(mid, w, bounds), mid-w/2; got != want {
		t.Errorf("a middle label starts at %v, want it centred at %v", got, want)
	}
}
