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

// SelectPipeline chooses the best pipeline based on scene statistics
// and GPU capabilities.
//
// Heuristics:
//   - Simple scenes (< 10 shapes, shallow clips): RenderPass is faster
//     (no encoding overhead, direct GPU draw calls)
//   - Complex scenes (> 50 shapes, deep clips, high overlap): Compute excels
//     (massively parallel tile-based processing)
//   - Text-heavy: RenderPass (MSDF Text tier is specialized)
//   - Default for medium complexity: Compute
//
// AutoSelectCompute controls whether PipelineModeAuto may choose the compute
// pipeline. It is false — on measurement, not on doubt about correctness.
//
// The compute path is correct: it matches the CPU rasterizer pixel for pixel on
// every scene in TestVelloComputeGolden, and those tests run whenever a GPU is
// present, independent of this flag. What it is not, on the evidence so far, is
// faster. Against the CPU rasterizer on this machine:
//
//	 256px,  16 paths   2.87ms vs  1.29ms   2.2x slower
//	 512px,  64 paths   6.05ms vs  4.77ms   1.3x slower
//	1024px, 256 paths   18.0ms vs  18.7ms   parity
//	2048px, 512 paths   65.6ms vs  66.7ms   parity
//
// SelectPipeline switches to compute above 50 shapes, which is the 512px row —
// where it is measurably slower. Defaulting to it there would trade a known
// cost for a hoped-for benefit.
//
// The measurement is not the last word, and the flag is not a verdict. The only
// headless harness for the compute path renders and then reads the whole
// framebuffer back to host memory — 16MB at 2048px — which a real frame would
// never pay, and the render-pass path it would actually replace needs a surface
// and cannot be benchmarked headlessly at all. So these numbers bound the
// compute path from below and say nothing yet about the comparison that
// matters. Until that comparison exists, the default stays where the evidence
// is.
//
// Set true to opt in; PipelineModeCompute always works when asked for
// explicitly. This gates only the automatic choice, never the heuristic:
// SelectPipeline stays a pure function of its inputs, so its tests describe the
// policy rather than the readiness of what the policy selects.
var AutoSelectCompute = false

func SelectPipeline(stats SceneStats, hasComputeSupport bool) PipelineMode {
	if !hasComputeSupport {
		return PipelineModeRenderPass
	}

	// Simple scenes: render pass is faster (no encoding overhead)
	if stats.ShapeCount < 10 && stats.ClipDepth < 2 {
		return PipelineModeRenderPass
	}

	// Complex scenes: compute excels
	if stats.ShapeCount > 50 || stats.ClipDepth > 3 || stats.OverlapFactor > 0.5 {
		return PipelineModeCompute
	}

	// Text-heavy: render pass (MSDF Text tier is specialized)
	if stats.TextCount > 0 {
		total := stats.ShapeCount + stats.TextCount
		if total > 0 {
			textRatio := float64(stats.TextCount) / float64(total)
			if textRatio > 0.6 {
				return PipelineModeRenderPass
			}
		}
	}

	// Default: compute for medium complexity
	return PipelineModeCompute
}
