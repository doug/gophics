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

	// rc is reused across frames, as a real renderer reuses one per surface.
	// Creating one per frame builds a fresh GPURenderSession each time — clip
	// buffers, convex buffers, bind groups — and charges both arms for setup
	// no application pays. It also leaks: a context that is never Closed
	// leaves its session to a GC finaliser, which is where the
	// "Buffer released by GC (missing explicit Release)" warnings came from.
	rc *GPURenderContext
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
	if h.rc != nil {
		h.rc.Close()
		h.rc = nil
	}
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

// harnessShape is one path with the paint it should be drawn with.
type harnessShape struct {
	path  *gg.Path
	paint *gg.Paint
	// clip, when set, is applied before this shape is drawn.
	clip *gg.Path
}

// renderOnce draws the scene through one pipeline mode and flushes it.
func (h *offscreenHarness) renderOnce(mode gg.PipelineMode, shapes []harnessShape) error {
	if h.rc == nil {
		h.rc = h.shared.NewRenderContext()
	}
	rc := h.rc
	rc.SetPipelineMode(mode)
	for _, s := range shapes {
		if s.clip != nil {
			rc.SetClipPath(s.clip)
		}
		if err := rc.FillPath(h.target, s.path, s.paint); err != nil {
			return err
		}
	}
	return rc.Flush(h.target)
}

// solidPaint returns an opaque paint in a deterministic colour.
func solidPaint(i int) *gg.Paint {
	p := gg.NewPaint()
	p.SetBrush(gg.Solid(gg.RGBA{
		R: float64(i%7) / 7, G: float64(i%5) / 5, B: float64(i%3) / 3, A: 1,
	}))
	return p
}

// harnessScene builds n small closed polygons spread over the canvas. This is
// the workload least suited to a compute rasterizer: small, disjoint, few.
func harnessScene(size, n int) []harnessShape {
	out := make([]harnessShape, 0, n)
	for i := 0; i < n; i++ {
		x := float64((i*37)%(size-40) + 10)
		y := float64((i*61)%(size-40) + 10)
		w := float64(20 + (i*13)%40)
		p := &gg.Path{}
		p.MoveTo(x, y)
		p.LineTo(x+w, y+w/2)
		p.LineTo(x, y+w)
		p.Close()
		out = append(out, harnessShape{path: p, paint: solidPaint(i)})
	}
	return out
}

// harnessOverlapScene builds n large translucent polygons that all cover the
// middle of the canvas. Every pixel there is touched by most of them, which is
// the case a tile-based rasterizer is supposed to handle in one pass while a
// render-pass pipeline pays for each layer separately.
func harnessOverlapScene(size, n int) []harnessShape {
	out := make([]harnessShape, 0, n)
	c := float64(size) / 2
	r := float64(size) * 0.42
	for i := 0; i < n; i++ {
		// Small offsets so the shapes pile up rather than tile.
		dx := float64((i%9)-4) * 3
		dy := float64((i%7)-3) * 3
		p := &gg.Path{}
		p.MoveTo(c-r+dx, c-r+dy)
		p.LineTo(c+r+dx, c-r+dy)
		p.LineTo(c+r+dx, c+r+dy)
		p.LineTo(c-r+dx, c+r+dy)
		p.Close()

		paint := gg.NewPaint()
		paint.SetBrush(gg.Solid(gg.RGBA{
			R: float64(i%7) / 7, G: float64(i%5) / 5, B: float64(i%3) / 3, A: 0.25,
		}))
		out = append(out, harnessShape{path: p, paint: paint})
	}
	return out
}

// harnessClipScene draws n shapes each under its own clip path. Clipping is
// where the two designs differ most: a compute pipeline carries clip state per
// tile through its command list, while a render-pass pipeline generally pays
// with stencil work per clip change.
func harnessClipScene(size, n int) []harnessShape {
	out := make([]harnessShape, 0, n)
	c := float64(size) / 2
	for i := 0; i < n; i++ {
		// Clip regions shrink toward the centre as i grows, so later shapes
		// are progressively more constrained.
		inset := float64(10 + (i%12)*4)
		clip := &gg.Path{}
		clip.MoveTo(inset, inset)
		clip.LineTo(float64(size)-inset, inset)
		clip.LineTo(float64(size)-inset, float64(size)-inset)
		clip.LineTo(inset, float64(size)-inset)
		clip.Close()

		r := float64(size) * 0.35
		dx := float64((i%11)-5) * 4
		p := &gg.Path{}
		p.MoveTo(c-r+dx, c-r)
		p.LineTo(c+r+dx, c)
		p.LineTo(c-r+dx, c+r)
		p.Close()

		out = append(out, harnessShape{path: p, paint: solidPaint(i), clip: clip})
	}
	return out
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

	// Every scene class the benchmark uses, because each has its own way of
	// producing nothing: a clip that excludes the shape, or translucency that
	// rounds to zero. Either would benchmark as beautifully fast.
	scenes := []struct {
		name  string
		build func(size, n int) []harnessShape
	}{
		{"disjoint", harnessScene},
		{"overlap", harnessOverlapScene},
		{"clipped", harnessClipScene},
	}

	for _, sc := range scenes {
		for _, mode := range []struct {
			name string
			mode gg.PipelineMode
		}{
			{"renderpass", gg.PipelineModeRenderPass},
			{"compute", gg.PipelineModeCompute},
		} {
			t.Run(sc.name+"/"+mode.name, func(t *testing.T) {
				h := newOffscreenHarness(t, size)
				defer h.close()

				if err := h.renderOnce(mode.mode, sc.build(size, 64)); err != nil {
					t.Fatalf("%s render: %v", mode.name, err)
				}
				if ink := countHarnessInk(t, h, size); ink == 0 {
					t.Fatalf("%s/%s drew nothing to the offscreen target", sc.name, mode.name)
				}
			})
		}
	}
}

// BenchmarkPipelineOffscreen is M12's comparison: both GPU pipelines, same
// scene, same offscreen target, no readback in either arm.
//
// Three workload classes, because the first comparison only ran the one a
// compute rasterizer is worst at. "disjoint" is many small separate shapes;
// "overlap" piles translucent shapes on the same pixels; "clipped" gives each
// shape its own clip path. The second and third are where a tile-based design
// is supposed to pull ahead — it resolves a tile's whole command list in one
// pass, while a render-pass pipeline pays per layer and per clip change.
func BenchmarkPipelineOffscreen(b *testing.B) {
	scenes := []struct {
		name  string
		build func(size, n int) []harnessShape
		count int
	}{
		{"disjoint", harnessScene, 256},
		{"disjoint2k", harnessScene, 2000},
		{"overlap", harnessOverlapScene, 64},
		{"overlap256", harnessOverlapScene, 256},
		{"clipped", harnessClipScene, 64},
		{"clipped256", harnessClipScene, 256},
	}

	for _, size := range []int{512, 1024} {
		for _, sc := range scenes {
			shapes := sc.build(size, sc.count)
			for _, mode := range []struct {
				name string
				mode gg.PipelineMode
			}{
				{"renderpass", gg.PipelineModeRenderPass},
				{"compute", gg.PipelineModeCompute},
			} {
				b.Run(fmt.Sprintf("%dpx_%s/%s", size, sc.name, mode.name), func(b *testing.B) {
					//nolint:gosec // bounded test sizes
					h := newOffscreenHarness(b, uint32(size))
					defer h.close()
					if err := h.renderOnce(mode.mode, shapes); err != nil {
						b.Skipf("%s unavailable: %v", mode.name, err)
					}
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := h.renderOnce(mode.mode, shapes); err != nil {
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
