//go:build !nogpu

package gpu

import (
	"context"
	"testing"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// TestComputePresentsToGPUDirectTarget renders through the compute path into a
// texture view and checks the pixels arrived.
//
// This target shape — a texture view with no CPU buffer — used to produce
// nothing at all. The compute path finished a frame by reading its whole
// output back to host memory and compositing in a CPU loop over target.Data,
// so with no Data the loop skipped every pixel and returned success. It is
// reachable through the public API: gpu_layers.go renders opacity layers into
// exactly this kind of target and passes the pipeline mode through.
//
// The test reads the texture back afterwards, which the renderer never does.
// That is the point: the readback belongs to the assertion, not the path under
// test, and the path is only correct if it never needed one.
func TestComputePresentsToGPUDirectTarget(t *testing.T) {
	a := &VelloAccelerator{}
	if err := a.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer a.Close()
	if !a.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	const size = 64
	tex, err := a.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "present_test_target",
		Size:          wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment |
			gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("create target texture: %v", err)
	}
	defer tex.Release()

	view, err := a.device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("create target view: %v", err)
	}
	defer view.Release()

	target := gg.GPURenderTarget{
		View:       gpucontext.NewTextureView(unsafe.Pointer(view)),
		ViewWidth:  size,
		ViewHeight: size,
		ViewFormat: gputypes.TextureFormatRGBA8Unorm,
		Width:      size,
		Height:     size,
	}

	p := &gg.Path{}
	p.MoveTo(8, 8)
	p.LineTo(56, 8)
	p.LineTo(56, 56)
	p.LineTo(8, 56)
	p.Close()

	if err := a.FillPath(target, p, gg.NewPaint()); err != nil {
		t.Fatalf("FillPath: %v", err)
	}
	if err := a.Flush(target); err != nil {
		t.Fatalf("Flush to a GPU-direct target: %v", err)
	}

	if ink := countTextureInk(t, a, tex, size); ink == 0 {
		t.Fatal("target texture is empty — the compute path reported success without drawing anything")
	}
}

// countTextureInk reads the texture back and counts non-transparent pixels.
func countTextureInk(t *testing.T, a *VelloAccelerator, tex *wgpu.Texture, size uint32) int {
	t.Helper()

	// 256-byte row alignment is required for texture-to-buffer copies.
	const bytesPerRow = 256 * ((64*4 + 255) / 256)
	staging, err := a.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "present_test_readback",
		Size:  uint64(bytesPerRow) * uint64(size),
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create readback buffer: %v", err)
	}
	defer staging.Release()

	enc, err := a.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "present_test_readback"})
	if err != nil {
		t.Fatalf("create encoder: %v", err)
	}
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: size},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("finish readback: %v", err)
	}
	if _, err := a.queue.Submit(cmd); err != nil {
		t.Fatalf("submit readback: %v", err)
	}

	total := uint64(bytesPerRow) * uint64(size)
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, total); err != nil {
		t.Fatalf("map readback: %v", err)
	}
	rng, err := staging.MappedRange(0, total)
	if err != nil {
		t.Fatalf("mapped range: %v", err)
	}
	data := append([]byte(nil), rng.Bytes()...)
	if err := staging.Unmap(); err != nil {
		t.Logf("unmap: %v", err)
	}

	ink := 0
	for y := uint32(0); y < size; y++ {
		row := data[y*bytesPerRow:]
		for x := uint32(0); x < size; x++ {
			if row[x*4+3] != 0 {
				ink++
			}
		}
	}
	return ink
}
