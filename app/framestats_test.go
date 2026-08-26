package app

import "testing"

// The pacing summary reports what the worst frame drew, not just how long it
// took.
//
// A frame time on its own cannot separate the two things that produce a spike.
// A frame that drew a much larger scene was simply heavier; a frame that drew a
// median-sized scene and still took four times as long hit a discrete event —
// a layer resolved, an atlas grown, a glyph rasterized for the first time.
// Those want completely different fixes, and "occasional stutter" was hard to
// act on precisely because the readout could not tell them apart.
func TestFrameStatsReportsTheWorstFrameScene(t *testing.T) {
	c := &core{}
	// Sixty ordinary frames drawing ordinary scenes, and one spike that drew
	// no more than the rest — the discrete-event shape.
	for i := range 59 {
		c.recordFrame(5, 100+i%3, 1)
	}
	c.recordFrame(40, 101, 1)

	f := c.FrameStats()
	if f.Worst != 40 {
		t.Errorf("worst is %v, want the 40ms spike", f.Worst)
	}
	if f.P50 > 6 {
		t.Errorf("p50 is %v; one spike should not move the median", f.P50)
	}
	if f.WorstOps != 101 {
		t.Errorf("worst frame drew %d ops, want the spike's own 101", f.WorstOps)
	}
	if f.MedianOps < 100 || f.MedianOps > 102 {
		t.Errorf("median scene is %d ops, want about 101", f.MedianOps)
	}
	// The diagnosis this enables: the spike's scene is the same size as the
	// median, so the scene did not get bigger — something else cost the time.
	if f.WorstOps > f.MedianOps*2 {
		t.Errorf("this fixture was meant to model a spike on an ordinary scene")
	}
}

func TestFrameStatsOnAnEmptyRing(t *testing.T) {
	if f := (&core{}).FrameStats(); f.Worst != 0 || f.WorstOps != 0 {
		t.Errorf("empty ring reported %+v, want zeros", f)
	}
}
