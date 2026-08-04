package gputypes

import "testing"

// TestBlendStatePreserveDst guards the blend used by stencil-only render passes.
// A stencil-fill pass must not alter destination color. WriteMask=None expresses
// that, but some WebGPU backends (Chrome/Dawn via the browser present path) were
// observed to ignore it, letting the fan fragment paint solid color over the
// whole shape — the "solitaire cards render black" bug. This blend (src=0, dst=1)
// makes the color a no-op regardless of the write mask: result = 0*src + 1*dst.
func TestBlendStatePreserveDst(t *testing.T) {
	b := BlendStatePreserveDst()
	for name, c := range map[string]BlendComponent{"color": b.Color, "alpha": b.Alpha} {
		if c.SrcFactor != BlendFactorZero {
			t.Errorf("%s SrcFactor = %v, want BlendFactorZero (so source is discarded)", name, c.SrcFactor)
		}
		if c.DstFactor != BlendFactorOne {
			t.Errorf("%s DstFactor = %v, want BlendFactorOne (so destination is kept)", name, c.DstFactor)
		}
		if c.Operation != BlendOperationAdd {
			t.Errorf("%s Operation = %v, want BlendOperationAdd", name, c.Operation)
		}
	}
}
