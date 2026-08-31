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

// The summary carries what the worst frame had to create.
//
// Scene size answers "did this frame draw more". It cannot answer the case the
// device actually shows — a 55ms frame that drew fewer ops than the median —
// because the work is not in the drawing. A pipeline compiled or a texture
// allocated the first time something is needed costs a frame and appears in no
// measure of what was drawn.
//
// The counters sit on Device.CreateTexture, CreateRenderPipeline, CreateBuffer
// and CreateBindGroup, which every such allocation passes through, so unlike a
// counter on one code path this cannot quietly measure nothing — which is
// exactly how the previous attempt at this failed, having instrumented a text
// mode nothing here runs.
func TestFrameStatsReportsWhatTheWorstFrameCreated(t *testing.T) {
	c := &core{}
	for range 59 {
		c.recordFrameMade(5, 100, 0, MadeCounts{}) // steady frames make nothing
	}
	// The spike, on a *smaller* scene: one pipeline compiled, six buffers and
	// bind groups churned — the shape the stencil tier used to produce.
	c.recordFrameMade(48, 99, 0, MadeCounts{Pipelines: 1, Buffers: 4, BindGroups: 2})

	f := c.FrameStats()
	if f.Worst != 48 {
		t.Errorf("worst is %v, want the 48ms spike", f.Worst)
	}
	if f.WorstMade.Total() != 7 {
		t.Errorf("worst frame made %d gpu objects, want 7", f.WorstMade.Total())
	}
	// The breakdown is the point: a total of 7 cannot distinguish one pipeline
	// compiled once from six buffers churned every frame, and only the second
	// is a bug.
	if f.WorstMade.Buffers != 4 || f.WorstMade.BindGroups != 2 {
		t.Errorf("worst frame breakdown = %+v, want 4 buffers and 2 bind groups", f.WorstMade)
	}
	if got := f.WorstMade.String(); got != "4 buffers + 2 bind groups + 1 pipeline" {
		t.Errorf("WorstMade.String() = %q", got)
	}
	if f.WorstOps > f.MedianOps {
		t.Errorf("this fixture models a spike on a *smaller* scene; got worst %d vs median %d",
			f.WorstOps, f.MedianOps)
	}
}
