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
	show := visibleXLabels(ticks, area, Axis{}, wide)

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

	for i, v := range visibleXLabels(ticks, area, Axis{}, narrow) {
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

	show := visibleXLabels(ticks, area, Axis{}, huge)
	if !show[0] {
		t.Error("no labels at all on a very narrow axis")
	}
}
