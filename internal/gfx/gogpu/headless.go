package gogpu

// Headless rendering support: a Renderer with a real GPU device but no window
// or surface. Combined with RenderToImage and GPUContextProvider it lets tests
// and batch tools drive the full GPU pipeline (including gg/ggcanvas
// compositing) off-screen and read the result back as an image, so rendering
// can be verified in CI and during development without a display.

import (
	"fmt"

	"github.com/doug/gophics/internal/gfx/gogpu/gpu/types"
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/internal/gfx/gputypes"
)

// NewHeadlessRenderer creates a Renderer backed by a GPU device but with no
// surface. It can render only via RenderToImage (which injects a synthetic
// off-screen RenderTarget). Returns an error — rather than skipping — when no
// GPU adapter/device is available, so callers decide how to handle it.
func NewHeadlessRenderer() (*Renderer, error) {
	r := &Renderer{
		powerPreference: gputypes.PowerPreferenceHighPerformance,
	}
	r.primary = &RenderTarget{renderer: r}

	if err := r.initInstance(types.GraphicsAPIAuto); err != nil {
		return nil, fmt.Errorf("gogpu: headless instance: %w", err)
	}
	if err := r.initAdapterDevice(nil); err != nil {
		return nil, fmt.Errorf("gogpu: headless adapter/device: %w", err)
	}
	// initAdapterDevice sets r.surfaceFormat but leaves the synthetic primary's
	// format unset; SurfaceFormat() (used by gg/ggcanvas) reads primary.format.
	r.primary.format = r.surfaceFormat
	return r, nil
}

// GPUContextProvider returns a gpucontext.DeviceProvider backed by this
// renderer, for use with gg/ggcanvas and other libraries. Unlike
// App.GPUContextProvider it has no App, so window-only queries (Size,
// ScaleFactor, clipboard, accessibility) return zero values — the render and
// device paths are fully functional.
func (r *Renderer) GPUContextProvider() gpucontext.DeviceProvider {
	return &gpuContextAdapter{renderer: r, tracker: &resourceTracker{}}
}
