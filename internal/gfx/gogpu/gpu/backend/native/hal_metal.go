//go:build darwin

package native

import (
	"github.com/doug/gossamer/internal/gfx/gogpu/gpu/types"
	"github.com/doug/gossamer/internal/gfx/gputypes"

	// Importing HAL backends triggers their init() registration with hal.RegisterBackend().
	_ "github.com/doug/gossamer/internal/gfx/wgpu/hal/metal"
	_ "github.com/doug/gossamer/internal/gfx/wgpu/hal/software"
)

// BackendInfo returns the backend display name and mask for the given graphics API.
func BackendInfo(api types.GraphicsAPI) (name string, mask gputypes.Backends) {
	switch api {
	case types.GraphicsAPISoftware:
		return "Pure Go (Software)", 0
	default: // Metal (default on macOS)
		return "Pure Go (Metal)", gputypes.BackendsMetal
	}
}
