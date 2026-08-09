//go:build windows

package native

import (
	"github.com/doug/gophics/internal/gfx/gogpu/gpu/types"
	"github.com/doug/gophics/internal/gfx/gputypes"

	// Importing HAL backends triggers their init() registration with hal.RegisterBackend().
	// This is required for wgpu.CreateInstance() to discover available backends.
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/gles"
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/software"
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/vulkan"
)

// BackendInfo returns the backend display name and mask for the given graphics API.
// For Auto mode, returns a multi-backend mask so wgpu can enumerate all available
// backends and pick the best adapter (Rust wgpu pattern). For explicit API selection,
// returns a single-backend mask.
//
// The DX12 HAL was removed from this tree (Windows renders via Vulkan); a
// GraphicsAPIDX12 request falls through to Auto, which prefers Vulkan with a
// GLES fallback.
func BackendInfo(api types.GraphicsAPI) (name string, mask gputypes.Backends) {
	switch api {
	case types.GraphicsAPIGLES:
		return "Pure Go (GLES)", gputypes.BackendsGL
	case types.GraphicsAPIVulkan:
		return "Pure Go (Vulkan)", gputypes.BackendsVulkan
	case types.GraphicsAPISoftware:
		return "Pure Go (Software)", 0 // software backend passes through mask filter
	default: // Auto (and legacy DX12 requests) — enumerate Vulkan, GLES; best GPU adapter wins
		return "Pure Go (Auto)", gputypes.BackendsVulkan | gputypes.BackendsGL
	}
}
