//go:build !nogpu

package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/scene"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// BackendGPU is the identifier for the GPU backend.
const BackendGPU = "gpu"

// Backend is a GPU-accelerated rendering backend using gogpu/wgpu.
//
// It manages GPU resources (instance, adapter, device, queue) through the
// portable gogpu/wgpu API, which has both a native and a browser (WebGPU)
// implementation — so the same GPU renderer runs on every platform, WASM
// included.
type Backend struct {
	mu sync.RWMutex

	// GPU resources (portable wgpu objects).
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue

	// GPU information
	gpuInfo *GPUInfo

	// State
	initialized bool
}

// NewBackend creates a new Pure Go GPU rendering backend.
// The backend must be initialized with Init() before use.
func NewBackend() *Backend {
	return &Backend{}
}

// Name returns the backend identifier.
func (b *Backend) Name() string {
	return BackendGPU
}

// Init initializes the backend by creating GPU resources.
// This includes creating an instance, requesting an adapter,
// creating a device, and getting the command queue.
//
// Returns an error if GPU initialization fails.
func (b *Backend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return nil
	}

	// Step 1: Create Instance (native HAL or browser navigator.gpu).
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: gputypes.BackendsPrimary,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGPU, err)
	}
	b.instance = inst

	// Step 2: Request Adapter (prefer high performance GPU).
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGPU, err)
	}
	b.adapter = adapter

	logGPUInfo(adapter)
	b.gpuInfo, _ = getGPUInfo(adapter)

	// Step 3: Create Device (nil descriptor → default features/limits).
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("device creation failed: %w", err)
	}
	b.device = device

	// Step 4: Get Queue.
	b.queue = device.Queue()

	b.initialized = true
	slogger().Debug("backend initialized")

	return nil
}

// Close releases all backend resources.
// The backend should not be used after Close is called.
func (b *Backend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return
	}

	// Release resources in reverse order of creation. The queue is owned by
	// the device and released with it.
	if b.device != nil {
		b.device.Release()
		b.device = nil
	}
	if b.adapter != nil {
		b.adapter.Release()
		b.adapter = nil
	}
	if b.instance != nil {
		b.instance.Release()
		b.instance = nil
	}
	b.queue = nil
	b.gpuInfo = nil
	b.initialized = false

	slogger().Debug("backend closed")
}

// NewRenderer creates a renderer for immediate mode rendering.
// The renderer is sized for the given dimensions.
//
// Note: This is a stub implementation that returns a GPURenderer.
// The actual GPU rendering will be implemented in TASK-110.
func (b *Backend) NewRenderer(width, height int) gg.Renderer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.initialized {
		slogger().Warn("creating renderer on uninitialized backend")
		return nil
	}

	if width <= 0 || height <= 0 {
		slogger().Warn("invalid dimensions", "width", width, "height", height)
		return nil
	}

	return newGPURenderer(b, width, height)
}

// RenderScene renders a scene to the target pixmap using retained mode.
// This method is optimized for complex scenes with many draw operations.
//
// The implementation uses GPUSceneRenderer for tessellation, strip
// rasterization, and layer compositing on the GPU. When wgpu texture
// readback is fully implemented, results will be downloaded to the target
// pixmap. Currently, data flows through the GPU pipeline as stubs.
func (b *Backend) RenderScene(target *gg.Pixmap, s *scene.Scene) error {
	b.mu.RLock()
	initialized := b.initialized
	b.mu.RUnlock()

	if !initialized {
		return ErrNotInitialized
	}

	if target == nil {
		return ErrNilTarget
	}

	if s == nil {
		return ErrNilScene
	}

	// Create GPU scene renderer for this frame
	renderer, err := NewGPUSceneRenderer(b, GPUSceneRendererConfig{
		Width:  target.Width(),
		Height: target.Height(),
	})
	if err != nil {
		return fmt.Errorf("failed to create GPU renderer: %w", err)
	}
	defer renderer.Close()

	// Render the scene to GPU
	if err := renderer.RenderToPixmap(target, s); err != nil {
		// ErrTextureReadbackNotSupported is expected until wgpu implements readback
		// In this case, the GPU pipeline was executed but we can't retrieve results
		if errors.Is(err, ErrTextureReadbackNotSupported) {
			// Log for debugging but don't fail - GPU ops were executed
			slogger().Debug("RenderScene completed on GPU", "note", "readback pending wgpu support")
			return nil
		}
		return fmt.Errorf("GPU render failed: %w", err)
	}

	return nil
}

// IsInitialized returns true if the backend has been initialized.
func (b *Backend) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized
}

// GPUInfo returns information about the selected GPU.
// Returns nil if the backend is not initialized.
func (b *Backend) GPUInfo() *GPUInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gpuInfo
}

// Device returns the GPU device, or nil if the backend is not initialized.
func (b *Backend) Device() *wgpu.Device {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.device
}

// Queue returns the GPU queue, or nil if the backend is not initialized.
func (b *Backend) Queue() *wgpu.Queue {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.queue
}

// GPURenderer is a GPU-backed renderer for immediate mode drawing.
// It implements the gg.Renderer interface.
//
// Note: This is a stub implementation. The actual GPU rendering
// will be implemented in TASK-110.
type GPURenderer struct {
	backend          *Backend
	width            int
	height           int
	softwareRenderer *gg.SoftwareRenderer
}

// newGPURenderer creates a new GPU renderer.
func newGPURenderer(b *Backend, width, height int) *GPURenderer {
	return &GPURenderer{
		backend:          b,
		width:            width,
		height:           height,
		softwareRenderer: gg.NewSoftwareRenderer(width, height),
	}
}

// Fill fills a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and gpu GPU path rendering.
func (r *GPURenderer) Fill(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Fill(pixmap, path, paint); err != nil {
		return fmt.Errorf("fill: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: GPU path tessellation

	return nil
}

// Stroke strokes a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and gpu GPU stroke expansion.
func (r *GPURenderer) Stroke(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Stroke(pixmap, path, paint); err != nil {
		return fmt.Errorf("stroke: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: GPU stroke expansion and tessellation

	return nil
}

// Width returns the renderer width.
func (r *GPURenderer) Width() int {
	return r.width
}

// Height returns the renderer height.
func (r *GPURenderer) Height() int {
	return r.height
}

// Close releases renderer resources.
// Note: This is a stub implementation.
func (r *GPURenderer) Close() {
	// TODO: Release GPU resources in TASK-110
}
