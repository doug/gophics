package wgpu

import "sync/atomic"

// Per-frame encoder activity: render passes begun, draws recorded, pipelines
// bound.
//
// Separate from DeviceStats because they answer a different question and have a
// different lifetime. Device counters are process-lifetime totals of things
// that are usually created once; these are per-frame work and are reset at each
// frame boundary.
//
// They live at the encoder entry points rather than at the renderer's call
// sites for the reason devicestats.go gives for its own: a counter on the choke
// point cannot miss a path, and a counter on one code path can. The renderer
// has seven BeginRenderPass sites and records draws from two loops that share
// no state, so instrumenting the callers would have been seven chances to be
// wrong and to look right.
//
// What they are for (design/rendering-pipeline.md):
//   - F2 predicts pipeline switches ≈ 2× the stencil-tier population, because
//     stencil-then-cover alternates two pipelines per path. That is falsifiable
//     only once both are counted.
//   - F3 argues from a pass count, and nothing counted passes. A frame that
//     splits into more than one pass is the corruption case confirmed on a
//     Pixel 10 Pro.
//
// PipelineSwitches counts every SetPipeline call, including a redundant rebind
// of the pipeline already bound — that redundancy is precisely what F2 is
// about, so collapsing it here would hide the finding.
var (
	renderPasses     atomic.Uint64
	drawCalls        atomic.Uint64
	pipelineSwitches atomic.Uint64
)

// EncoderCounts is one frame's encoder activity.
type EncoderCounts struct {
	RenderPasses     uint64
	DrawCalls        uint64
	PipelineSwitches uint64
}

// EncoderStats returns the counters as they stand.
func EncoderStats() EncoderCounts {
	return EncoderCounts{
		RenderPasses:     renderPasses.Load(),
		DrawCalls:        drawCalls.Load(),
		PipelineSwitches: pipelineSwitches.Load(),
	}
}

// ResetEncoderStats zeroes them at a frame boundary.
func ResetEncoderStats() {
	renderPasses.Store(0)
	drawCalls.Store(0)
	pipelineSwitches.Store(0)
}
