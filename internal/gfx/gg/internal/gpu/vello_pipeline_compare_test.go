//go:build !nogpu

package gpu

import (
	"context"
	"fmt"
	"testing"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// offscreenHarness drives a GPURenderContext to a texture view with no window,
// no surface and no readback — the comparison M12 needs and could not have
// until the compute path learned to composite onto a view.
//
// Both pipelines get the same context type, the same scene and the same
// target. The only variable is the pipeline mode.
type offscreenHarness struct {
	shared *GPUShared
	tex    *wgpu.Texture
	view   *wgpu.TextureView
	target gg.GPURenderTarget
}

func newOffscreenHarness(tb testing.TB, size uint32) *offscreenHarness {
	tb.Helper()

	shared := NewGPUShared()
	shared.mu.Lock()
	err := shared.initGPU()
	shared.mu.Unlock()
	if err != nil {
		tb.Skipf("GPU not available: %v", err)
	}

	dev := shared.Device()
	if dev == nil {
		shared.Close()
		tb.Skip("no device")
	}

	tex, err := dev.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "offscreen_target",
		Size:          wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment |
			gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		shared.Close()
		tb.Skipf("create offscreen target: %v", err)
	}
	view, err := dev.CreateTextureView(tex, nil)
	if err != nil {
		tex.Release()
		shared.Close()
		tb.Skipf("create offscreen view: %v", err)
	}

	return &offscreenHarness{
		shared: shared,
		tex:    tex,
		view:   view,
		target: gg.GPURenderTarget{
			View:       gpucontext.NewTextureView(unsafe.Pointer(view)),
			ViewWidth:  size,
			ViewHeight: size,
			ViewFormat: gputypes.TextureFormatRGBA8Unorm,
			//nolint:gosec // bounded test sizes
			Width: int(size), Height: int(size),
		},
	}
}

func (h *offscreenHarness) close() {
	if h.view != nil {
		h.view.Release()
	}
	if h.tex != nil {
		h.tex.Release()
	}
	if h.shared != nil {
		h.shared.Close()
	}
}

// renderOnce draws the scene through one pipeline mode and flushes it.
func (h *offscreenHarness) renderOnce(mode gg.PipelineMode, paths []*gg.Path) error {
	rc := h.shared.NewRenderContext()
	rc.SetPipelineMode(mode)
	paint := gg.NewPaint()
	for _, p := range paths {
		if err := rc.FillPath(h.target, p, paint); err != nil {
			return err
		}
	}
	return rc.Flush(h.target)
}

// harnessScene builds n closed polygons spread over the canvas.
func harnessScene(size, n int) []*gg.Path {
	paths := make([]*gg.Path, 0, n)
	for i := 0; i < n; i++ {
		x := float64((i*37)%(size-40) + 10)
		y := float64((i*61)%(size-40) + 10)
		w := float64(20 + (i*13)%40)
		p := &gg.Path{}
		p.MoveTo(x, y)
		p.LineTo(x+w, y+w/2)
		p.LineTo(x, y+w)
		p.Close()
		paths = append(paths, p)
	}
	return paths
}

// TestOffscreenHarnessDrivesBothPipelines checks the harness itself before any
// timing is read from it.
//
// A benchmark whose arms silently do nothing reports excellent numbers, and
// this pipeline has produced exactly that failure twice already — stub
// pipelines returning success, and a compute path compositing into a buffer
// that was not there. So the harness has to prove both modes reach the target
// before it is allowed to compare them.
func TestOffscreenHarnessDrivesBothPipelines(t *testing.T) {
	const size = 256
	scene := harnessScene(size, 64)

	for _, mode := range []struct {
		name string
		mode gg.PipelineMode
	}{
		{"renderpass", gg.PipelineModeRenderPass},
		{"compute", gg.PipelineModeCompute},
	} {
		t.Run(mode.name, func(t *testing.T) {
			h := newOffscreenHarness(t, size)
			defer h.close()

			if err := h.renderOnce(mode.mode, scene); err != nil {
				t.Fatalf("%s render: %v", mode.name, err)
			}
			if ink := countHarnessInk(t, h, size); ink == 0 {
				t.Fatalf("%s drew nothing to the offscreen target", mode.name)
			}
		})
	}
}

// BenchmarkPipelineOffscreen is M12's comparison: both GPU pipelines, same
// scene, same offscreen target, no readback in either arm.
func BenchmarkPipelineOffscreen(b *testing.B) {
	for _, size := range []int{256, 512, 1024} {
		for _, shapes := range []int{16, 64, 256} {
			scene := harnessScene(size, shapes)
			for _, mode := range []struct {
				name string
				mode gg.PipelineMode
			}{
				{"renderpass", gg.PipelineModeRenderPass},
				{"compute", gg.PipelineModeCompute},
			} {
				b.Run(fmt.Sprintf("%dpx_%dshapes/%s", size, shapes, mode.name), func(b *testing.B) {
					//nolint:gosec // bounded test sizes
					h := newOffscreenHarness(b, uint32(size))
					defer h.close()
					if err := h.renderOnce(mode.mode, scene); err != nil {
						b.Skipf("%s unavailable: %v", mode.name, err)
					}
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := h.renderOnce(mode.mode, scene); err != nil {
							b.Fatalf("render: %v", err)
						}
					}
				})
			}
		}
	}
}

// countHarnessInk reads the offscreen target back and counts non-transparent
// pixels. Used only to prove an arm draws — never inside a timed loop.
func countHarnessInk(t *testing.T, h *offscreenHarness, size uint32) int {
	t.Helper()
	dev := h.shared.Device()
	bytesPerRow := (size*4 + 255) / 256 * 256

	staging, err := dev.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "harness_readback",
		Size:  uint64(bytesPerRow) * uint64(size),
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("readback buffer: %v", err)
	}
	defer staging.Release()

	enc, err := dev.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "harness_readback"})
	if err != nil {
		t.Fatalf("encoder: %v", err)
	}
	enc.CopyTextureToBuffer(h.tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: size},
		TextureBase:  wgpu.ImageCopyTexture{Texture: h.tex},
		Size:         wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := h.shared.Queue().Submit(cmd); err != nil {
		t.Fatalf("submit: %v", err)
	}

	total := uint64(bytesPerRow) * uint64(size)
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, total); err != nil {
		t.Fatalf("map: %v", err)
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
