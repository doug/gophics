//go:build !nogpu

package gpu

import (
	"fmt"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg/internal/gpu/tilecompute"
)

// BenchmarkComputeVsCPU measures the compute vector pipeline against the CPU
// rasterizer on the scenes both are known to agree on pixel-for-pixel.
//
// What this does and does not measure. The compute figure is a whole frame:
// buffer allocation, scene upload, eight dispatches, and a full framebuffer
// readback to host memory. A real frame would keep the result on the GPU and
// composite it, so the readback is overhead this benchmark pays and a renderer
// would not — it is the honest number for "render a scene and get pixels back",
// not for "render a scene".
//
// The scenes are also small (100×100 and under) and simple, which is the case
// that favours the CPU: a compute pipeline's fixed costs are amortised by
// complexity, and these have almost none to amortise. Read this as a floor on
// the compute path rather than a verdict on it.
func BenchmarkComputeVsCPU(b *testing.B) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		b.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()
	if !accel.CanCompute() {
		b.Skip("compute pipeline not available")
	}

	for _, tc := range computeGoldenTests() {
		tc := tc
		b.Run(tc.Name+"/gpu_compute", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := accel.RenderSceneCompute(tc.Width, tc.Height, tc.BgColor, tc.Paths); err != nil {
					b.Fatalf("render: %v", err)
				}
			}
		})
		b.Run(tc.Name+"/cpu", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r := tilecompute.NewRasterizer(tc.Width, tc.Height)
				_ = r.RasterizeScenePTCL(tc.BgColor, tc.Paths)
			}
		})
	}
}

// benchScene builds a scene of nPaths triangles spread over a size×size canvas.
// Deterministic, so both paths rasterise identical geometry, and parameterised
// because the interesting question is not which path is faster on one scene but
// where the crossover is: a compute pipeline trades a fixed per-frame cost for
// parallelism, so it should lose on small scenes and win as they grow.
func benchScene(size, nPaths int) []tilecompute.PathDef {
	paths := make([]tilecompute.PathDef, 0, nPaths)
	for i := 0; i < nPaths; i++ {
		// A cheap deterministic spread; no RNG so runs are comparable.
		x := float32((i*37)%(size-40) + 10)
		y := float32((i*61)%(size-40) + 10)
		w := float32(20 + (i*13)%40)
		paths = append(paths, tilecompute.PathDef{
			Lines:    computeTriangleLines(x, y, x+w, y+w/2, x, y+w),
			Color:    [4]uint8{uint8(i * 7 % 256), uint8(i * 13 % 256), uint8(i * 29 % 256), 255},
			FillRule: tilecompute.FillRuleNonZero,
		})
	}
	return paths
}

// BenchmarkComputeScaling looks for the crossover point between the compute
// pipeline and the CPU rasterizer as scene size and path count grow. The
// compute figure still includes a full framebuffer readback, which a real
// frame would not pay.
func BenchmarkComputeScaling(b *testing.B) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		b.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()
	if !accel.CanCompute() {
		b.Skip("compute pipeline not available")
	}

	bg := [4]uint8{255, 255, 255, 255}
	for _, cfg := range []struct{ size, paths int }{
		{256, 16}, {512, 64}, {1024, 256}, {2048, 512},
	} {
		cfg := cfg
		scene := benchScene(cfg.size, cfg.paths)
		name := fmt.Sprintf("%dpx_%dpaths", cfg.size, cfg.paths)
		b.Run(name+"/gpu_compute", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := accel.RenderSceneCompute(cfg.size, cfg.size, bg, scene); err != nil {
					b.Fatalf("render: %v", err)
				}
			}
		})
		b.Run(name+"/cpu", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r := tilecompute.NewRasterizer(cfg.size, cfg.size)
				_ = r.RasterizeScenePTCL(bg, scene)
			}
		})
	}
}
