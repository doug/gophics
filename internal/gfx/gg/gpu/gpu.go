//go:build !nogpu

// Package gpu registers the GPU accelerator and coverage filler for
// hardware-accelerated rendering.
//
// Import this package to enable both GPU-based shape rendering (SDF for circles,
// rounded rectangles) and adaptive tile-based rasterization for complex paths.
// The GPU accelerator uses wgpu/hal compute shaders for parallel evaluation.
//
// If GPU initialization fails (no Vulkan/Metal/DX12 available), the
// registration is silently skipped and rendering falls back to CPU.
//
// For tile-based rasterization only (no GPU shapes), use:
//
//	import _ "github.com/doug/gossamer/internal/gfx/gg/raster"
//
// Usage:
//
//	import _ "github.com/doug/gossamer/internal/gfx/gg/gpu" // enable GPU acceleration + tile rasterization
package gpu

import (
	"github.com/doug/gossamer/internal/gfx/gg"
	gpuimpl "github.com/doug/gossamer/internal/gfx/gg/internal/gpu"
	"github.com/doug/gossamer/internal/gfx/gpucontext"
)

func init() {
	// GPU accelerator (SDF shapes: circles, rounded rects)
	accel := &gpuimpl.SDFAccelerator{}
	if err := gg.RegisterAccelerator(accel); err != nil {
		gg.Logger().Warn("GPU accelerator not available", "err", err)
	}

	// Coverage filler: AdaptiveFiller auto-selects between SparseStrips (4x4
	// tiles, SIMD-optimized) and TileCompute (16x16 tiles) based on path
	// complexity and canvas size.
	gg.RegisterCoverageFiller(&gpuimpl.AdaptiveFiller{})
}

// SetDeviceProvider configures the GPU accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). This avoids creating a separate
// GPU instance and enables efficient device sharing.
//
// The provider should be a gpucontext.DeviceProvider. The accelerator
// type-asserts provider.Device() to *wgpu.Device for HAL access.
//
// Call this before drawing operations, typically from ggcanvas.New() or
// manually after registering the accelerator.
func SetDeviceProvider(provider gpucontext.DeviceProvider) error {
	return gg.SetAcceleratorDeviceProvider(provider)
}
