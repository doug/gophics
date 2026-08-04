//go:build js && wasm

package native

import (
	"github.com/doug/gossamer/internal/gfx/gogpu/gpu/types"
	"github.com/doug/gossamer/internal/gfx/gputypes"
)

// BackendInfo returns metadata for the browser WebGPU backend.
// On browser, wgpu uses navigator.gpu directly — no HAL backend registration
// is needed. The Backends field in InstanceDescriptor is ignored.
func BackendInfo(_ types.GraphicsAPI) (name string, mask gputypes.Backends) {
	return "Browser WebGPU", 0
}
