//go:build !nogpu

package gpu

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
)

// TestComputeFallsBackOnGPUDirectTarget pins that the compute path refuses a
// target it cannot draw into, rather than reporting success and drawing
// nothing.
//
// The compute path dispatches, reads the whole framebuffer back to host
// memory, and composites in a CPU loop over target.Data. A GPU-direct target
// carries a texture view and no Data, so that loop skipped every pixel and
// returned nil — a blank result with no error anywhere.
//
// It is reachable through the public API: gpu_layers.go renders opacity layers
// into a view-only target and passes the pipeline mode straight through, and
// gg.PipelineModeCompute is documented for callers to select. So an app asking
// for compute got empty opacity layers and nothing said why.
//
// Falling back is the fix rather than the workaround — the render-pass path
// draws these correctly. Making compute write to a texture view on the GPU is
// the real repair, and is what M12 turns on.
func TestComputeFallsBackOnGPUDirectTarget(t *testing.T) {
	a := &VelloAccelerator{}
	if err := a.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer a.Close()
	if !a.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	// A non-nil view stands in for a real texture: the check under test looks
	// at whether a CPU buffer exists, and never dereferences the handle.
	var placeholder byte
	target := gg.GPURenderTarget{
		View:       gpucontext.NewTextureView(unsafe.Pointer(&placeholder)),
		ViewWidth:  64,
		ViewHeight: 64,
		Width:      64,
		Height:     64,
	}

	p := &gg.Path{}
	p.MoveTo(8, 8)
	p.LineTo(56, 8)
	p.LineTo(56, 56)
	p.Close()
	paint := gg.NewPaint()

	if err := a.FillPath(target, p, paint); err != nil && !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Fatalf("FillPath: %v", err)
	}

	err := a.Flush(target)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Fatalf("flushing to a GPU-direct target returned %v; want ErrFallbackToCPU "+
			"— this path can only composite into host memory, so success here means it drew nothing", err)
	}
}
