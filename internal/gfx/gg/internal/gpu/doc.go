//go:build !nogpu

// Package gpu provides a Pure Go GPU-accelerated rendering backend.
//
// This is an internal package used by the gg library for GPU rendering.
// It leverages WebGPU for hardware-accelerated 2D graphics rendering
// via the gogpu/wgpu Pure Go WebGPU implementation (zero CGO), which supports
// Vulkan, Metal, and DX12 backends depending on the platform.
//
// # Architecture Overview
//
// Scene rendering is a render-pass pipeline that sorts draws into tiers and
// records them in a fixed order per scissor group: SDF shapes, convex polygons,
// stencil-then-cover for everything else, images, and two text tiers. See
// GPURenderSession.
//
// Key components:
//
//   - Backend: Main entry point for GPU rendering
//   - GPURenderContext / GPURenderSession: tier routing and pass recording
//   - VelloAccelerator: the compute path, reachable via PipelineModeCompute
//   - MemoryManager: GPU texture memory with LRU eviction (configurable budget)
//   - TextureAtlas: Shelf-packing for efficient GPU memory usage
//
// This section used to describe a three-stage vello-style HybridPipeline
// (Flatten, Coarse, Fine) as *the* scene-rendering path. That pipeline was
// reachable only from its own tests and has been removed; the description
// outlived the code by long enough to be the most misleading page in the
// package. The measured compute path that remains is VelloAccelerator, which
// SelectPipeline does not currently choose: a measured table has the render
// pass winning every cell by 1.4x-7x on Metal.
//
// # Blend Modes
//
// All 29 standard blend modes are supported via WGSL shaders:
//
// Standard modes:
//   - Normal, Multiply, Screen, Overlay
//   - Darken, Lighten, ColorDodge, ColorBurn
//   - HardLight, SoftLight, Difference, Exclusion
//
// HSL modes:
//   - Hue, Saturation, Color, Luminosity
//
// Porter-Duff compositing:
//   - Clear, Copy, Destination
//   - SourceOver, DestinationOver
//   - SourceIn, DestinationIn
//   - SourceOut, DestinationOut
//   - SourceAtop, DestinationAtop
//   - Xor, Plus
//
// # Usage
//
// Create and initialize the gpu backend directly:
//
//	b := gpu.NewBackend()
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// # Memory Management
//
// The backend uses an LRU-based memory manager with configurable budget:
//
//	config := MemoryManagerConfig{
//	    BudgetMB: 256,
//	}
//
// When memory budget is exceeded, least-recently-used textures are evicted.
//
// # Requirements
//
//   - Go 1.25+ (for generic features)
//   - gogpu/wgpu module (github.com/doug/wgpu)
//   - A GPU that supports Vulkan, Metal, or DX12 (for actual GPU rendering)
//
// # Thread Safety
//
// Backend is safe for concurrent use from multiple
// goroutines. Internal synchronization is handled via mutexes.
//
// # Error Handling
//
// Common errors returned by this package:
//
//   - ErrNotInitialized: Backend must be initialized before use
//   - ErrNoGPU: No compatible GPU found
//   - ErrDeviceLost: GPU device was lost (requires re-initialization)
//   - ErrNilTarget: Target pixmap is nil
//   - ErrNilScene: Scene is nil
//
// # Benchmarking
//
// Run benchmarks to compare GPU vs Software performance:
//
//	go test -bench=. ./internal/gpu/...
//
// # Related Packages
//
//   - github.com/doug/gg: Core 2D graphics library
//   - github.com/doug/gg/scene: Scene graph and retained mode API
//   - github.com/doug/wgpu: Pure Go WebGPU implementation
//
// # References
//
//   - W3C WebGPU Specification: https://www.w3.org/TR/webgpu/
//   - gogpu Organization: https://github.com/gogpu
//   - gogpu/wgpu: https://github.com/doug/wgpu
package gpu
