//go:build !(js && wasm)

package software

import "github.com/doug/gophics/internal/gfx/wgpu/hal"

// init registers the software backend with the HAL registry.
func init() {
	hal.RegisterBackend(API{})
}
