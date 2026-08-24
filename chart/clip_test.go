package chart

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A tooltip has to fit inside the area it is given, from every anchor.
//
// The placement preferred above-left, clamped only the left edge, and flipped
// an upward overflow downward without checking where that landed. So a card
// could hang off the bottom, and off the top when the flip pushed it back out.
// That matters more than it sounds: a chart's selection is drawn after the
// plot clip is popped, so anything escaping the area painted onto the page
// around it — on a phone, over the header above a scrolled chart.
//
// Anchors are walked across and beyond the area, including outside it, because
// a scrolled chart genuinely reports anchors outside its visible box.
func TestTooltipStaysInsideItsArea(t *testing.T) {
	area := geom.RectXYWH(100, 200, 300, 180)
	const tw, th = 120, 60

	for _, anchor := range []geom.Pt{
		{X: 110, Y: 210}, // top-left corner
		{X: 390, Y: 210}, // top-right
		{X: 110, Y: 370}, // bottom-left
		{X: 390, Y: 370}, // bottom-right
		{X: 250, Y: 205}, // hard against the top edge
		{X: 250, Y: 378}, // hard against the bottom edge
		{X: 250, Y: 290}, // middle
		{X: 60, Y: 150},  // above and left of the area entirely
		{X: 460, Y: 430}, // below and right of it
	} {
		box := tooltipBox(area, anchor, tw, th)
		if box.Min.X < area.Min.X || box.Min.Y < area.Min.Y ||
			box.Max.X > area.Max.X || box.Max.Y > area.Max.Y {
			t.Errorf("anchor %v put the tooltip at %v, outside %v — a chart's "+
				"selection is drawn unclipped by the plot, so this lands on the page",
				anchor, box, area)
		}
		if box.Dx() != tw || box.Dy() != th {
			t.Errorf("anchor %v resized the tooltip to %.0fx%.0f, want %dx%d",
				anchor, box.Dx(), box.Dy(), tw, th)
		}
	}
}

// An area smaller than the tooltip cannot contain it; the card should pin to
// the top-left rather than wander.
func TestTooltipInATinyAreaPinsRatherThanWanders(t *testing.T) {
	area := geom.RectXYWH(0, 0, 50, 40)
	box := tooltipBox(area, geom.Pt{X: 25, Y: 20}, 120, 60)
	if box.Min.X != area.Min.X || box.Min.Y != area.Min.Y {
		t.Errorf("tooltip in an area too small for it sits at %v; want it pinned to %v",
			box.Min, area.Min)
	}
}
