package wgpu

import "sync/atomic"

// Counters for the device resources whose creation is slow enough to show up
// as a dropped frame.
//
// A frame time on its own says a frame was slow. Paired with the size of the
// scene it drew, it says whether the scene got bigger. Neither answers the
// case actually seen on a device: a 55ms frame that drew *fewer* ops than the
// median. What is left is work that happens once, the first time something is
// needed — a pipeline compiled, a texture allocated — and that is invisible in
// every measure of what was drawn.
//
// These are the choke points every such allocation passes through, so a
// counter here cannot miss one the way a counter on a specific code path can.
var (
	texturesCreated  atomic.Uint64
	pipelinesCreated atomic.Uint64
)

// DeviceStats reports how many textures and render pipelines have been created
// since the process started. Take a difference across a frame to see what that
// frame had to make.
func DeviceStats() (textures, pipelines uint64) {
	return texturesCreated.Load(), pipelinesCreated.Load()
}
