package gpu

import "github.com/doug/gophics/internal/gfx/gputypes"

// A render pipeline is compiled against the format of the attachment it will
// draw into, and WebGPU rejects a pass whose pipeline disagrees with its color
// target. Most desktop surfaces prefer BGRA8Unorm, so that was hardcoded
// throughout — but PowerVR parts (Pixel phones) offer RGBA8Unorm, and there
// every pass failed validation and the canvas stayed blank.
//
// The format is a property of the one surface a session presents into, so it
// lives here rather than being threaded through twenty pipeline constructors.
// The session sets it from the render target before any draw is encoded, and
// pipelines read it as they are built.
var surfaceColorFormat = gputypes.TextureFormatBGRA8Unorm

// SetSurfaceColorFormat records the format pipelines should target. Zero
// restores the BGRA8Unorm default. It reports whether the value changed, so a
// caller can drop pipelines compiled against the old format.
func SetSurfaceColorFormat(f gputypes.TextureFormat) bool {
	if f == 0 {
		f = gputypes.TextureFormatBGRA8Unorm
	}
	if f == surfaceColorFormat {
		return false
	}
	surfaceColorFormat = f
	return true
}

// SurfaceColorFormat is the format pipelines target.
func SurfaceColorFormat() gputypes.TextureFormat { return surfaceColorFormat }
