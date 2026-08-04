//go:build !(js && wasm)

package software

import "github.com/doug/gossamer/internal/gfx/wgpu/hal"

// init registers the software backend with the HAL registry.
func init() {
	hal.RegisterBackend(API{})
}
