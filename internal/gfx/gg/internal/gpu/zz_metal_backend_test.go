//go:build darwin && !nogpu

package gpu

// Register the wgpu HAL backends for this test binary so createMetalDevice's
// BackendsMetal request resolves to the real Apple GPU. Without a registered
// backend, wgpu falls back to a software adapter that does not support MSAA,
// which made the TestMetalStencil* tests fail with "software backend does not
// support MSAA". (Production binaries get backends via the shell; test binaries
// must import them explicitly.)
import _ "github.com/doug/gossamer/internal/gfx/wgpu/hal/allbackends"
