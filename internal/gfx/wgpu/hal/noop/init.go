//go:build !(js && wasm)

package noop

import "github.com/doug/gossamer/internal/gfx/wgpu/hal"

// init registers the noop backend with the HAL registry.
func init() {
	hal.RegisterBackend(API{})
}
