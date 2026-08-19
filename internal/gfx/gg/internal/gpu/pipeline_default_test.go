//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
)

// The default pipeline is render-pass, and it is the default by construction
// rather than by a flag someone can flip. A GPURenderContext's zero value is
// PipelineModeAuto, so this is what every shell that never calls
// SetPipelineMode gets -- desktop, notably, sets nothing at all.
//
// This asserts on effectivePipelineMode rather than on SelectPipeline because
// that is the function the flush path actually calls; a policy change that kept
// SelectPipeline honest but rewired the caller would pass the gg-level tests
// and still ship compute.
func TestAutoPipelineResolvesToRenderPass(t *testing.T) {
	var rc GPURenderContext // zero value: pipelineMode == PipelineModeAuto

	if rc.pipelineMode != gg.PipelineModeAuto {
		t.Fatalf("zero-value pipelineMode = %v, want Auto", rc.pipelineMode)
	}

	// Scene stats that the retired heuristic would have sent to compute.
	rc.sceneStats = gg.SceneStats{ShapeCount: 5000, ClipDepth: 32, OverlapFactor: 0.9}

	if got := rc.effectivePipelineMode(); got != gg.PipelineModeRenderPass {
		t.Errorf("effectivePipelineMode() in Auto = %v, want RenderPass", got)
	}
}

// Auto resolving to render-pass must not make compute unreachable -- the point
// of keeping the compute path is that asking for it still works.
func TestExplicitComputeModeIsHonored(t *testing.T) {
	var rc GPURenderContext
	rc.SetPipelineMode(gg.PipelineModeCompute)

	if got := rc.effectivePipelineMode(); got != gg.PipelineModeCompute {
		t.Errorf("effectivePipelineMode() after SetPipelineMode(Compute) = %v, want Compute", got)
	}

	rc.SetPipelineMode(gg.PipelineModeRenderPass)
	if got := rc.effectivePipelineMode(); got != gg.PipelineModeRenderPass {
		t.Errorf("effectivePipelineMode() after SetPipelineMode(RenderPass) = %v, want RenderPass", got)
	}
}
