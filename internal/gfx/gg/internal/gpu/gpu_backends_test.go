//go:build !nogpu

package gpu

// Register the platform's GPU backends for tests in this package.
//
// Backends are wired by blank-importing hal/allbackends, which only
// shell/mobile does in production. A test binary that omits it gets whatever
// happens to be registered otherwise — on Android that is nothing, so
// RequestAdapter returned a "Software Renderer" with backend=Empty and the
// compute pipeline failed to build against its 8-storage-buffer limit. That
// looked exactly like a PowerVR hardware constraint and was not: Vulkan loads
// on the device without complaint.
//
// Importing it here makes the tests exercise the same backends an application
// does, on every platform.
import _ "github.com/doug/gophics/internal/gfx/wgpu/hal/allbackends"
