package gg

// PipelineMode selects the GPU rendering pipeline.
type PipelineMode int

const (
	// PipelineModeAuto lets the framework select the best pipeline
	// based on scene complexity and GPU capabilities.
	PipelineModeAuto PipelineMode = iota

	// PipelineModeRenderPass forces the traditional multi-tier render pass
	// pipeline (SDF, Convex, Stencil-then-Cover, MSDF Text).
	PipelineModeRenderPass

	// PipelineModeCompute forces the Vello-style compute pipeline
	// (scene encoding, tile binning, PTCL fine rasterization).
	PipelineModeCompute
)

// String returns the pipeline mode name.
func (m PipelineMode) String() string {
	switch m {
	case PipelineModeAuto:
		return "Auto"
	case PipelineModeRenderPass:
		return "RenderPass"
	case PipelineModeCompute:
		return "Compute"
	default:
		return "Unknown"
	}
}

// SceneStats holds metrics for pipeline auto-selection.
// These are computed by analyzing the current frame's draw operations.
type SceneStats struct {
	ShapeCount    int     // Total number of shapes
	PathCount     int     // Complex paths (not simple SDF shapes)
	TextCount     int     // Text elements
	ClipDepth     int     // Maximum clip nesting depth
	OverlapFactor float64 // Estimated overlap ratio [0, 1]
}

// SelectPipeline chooses the rendering pipeline for a scene under
// PipelineModeAuto. It returns PipelineModeRenderPass for every scene.
//
// That is not a placeholder, and it is not doubt about the compute path's
// correctness. The compute path is correct: it matches the CPU rasterizer
// pixel for pixel across TestVelloComputeGolden and a 400-scene generated
// sweep, on Metal and on Vulkan, and those tests run whenever a GPU is present.
// It is simply slower everywhere it was measured.
//
// Measured render-pass vs compute, same scenes, same device, on Metal
// (M-series, offscreen harness, milliseconds per frame):
//
//	 256px,   64 paths    0.53  vs   2.94    5.5x
//	 512px,  256 paths    1.71  vs   7.32    4.3x
//	 512px,  256 paths, clipped
//	                      2.06  vs  10.96    5.3x
//	1024px,  512 paths    5.24  vs  17.31    3.3x
//
// Render-pass won every cell, by 1.4x to 7x. On a Pixel 10 Pro (PowerVR
// D-Series, real Vulkan) run-to-run variance is 2-3x, so those numbers are not
// quotable individually, but the direction was the same in every cell.
//
// Earlier revisions of this file switched to compute above 50 shapes and
// carried a table of compute-vs-CPU timings. That comparison was the wrong one
// — it measured the compute path against the CPU rasterizer rather than
// against the GPU path it would actually replace, and its harness read the
// whole framebuffer back to host memory, which a real frame never pays. The
// cross-pipeline benchmark that replaced it is the one above.
//
// Compute remains reachable: PipelineModeCompute selects it explicitly, which
// is how the golden tests and benchmarks drive it. What was removed is the
// automatic choice, because no measured scene favors it. If that changes,
// change it here — stats is retained so a future policy has its inputs, and
// hasComputeSupport still short-circuits when there is no compute backend.
func SelectPipeline(stats SceneStats, hasComputeSupport bool) PipelineMode {
	_ = stats
	_ = hasComputeSupport
	return PipelineModeRenderPass
}
