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
//
// Buffers and bind groups are counted for a sharper reason than textures and
// pipelines were. Those two are mostly first-use costs that amortize; buffers
// and bind groups are created and destroyed *every frame* by tier 2b, six per
// path, and until they were counted that
// finding was invisible to every instrument in the repo — including the
// on-device pacing readout, which reported "made N gpu objects" without ever
// being able to see the N that mattered.
//
// Caveat, because these are process-wide: they are differenced across a frame
// by whoever is measuring, so a second context or window creating resources
// concurrently attributes that work to this frame. Correct for the
// single-window measurement corpus, wrong as general accounting.
var (
	texturesCreated   atomic.Uint64
	pipelinesCreated  atomic.Uint64
	buffersCreated    atomic.Uint64
	bindGroupsCreated atomic.Uint64
)

// DeviceCounts is how many device resources have been created, by kind.
//
// A struct rather than a widening tuple: DeviceStats returned
// (textures, pipelines) and every added counter changed its signature and
// every call site. This makes that the last such change — the next counter is
// a field.
type DeviceCounts struct {
	Textures   uint64
	Pipelines  uint64
	Buffers    uint64
	BindGroups uint64
}

// Sub returns the resources created between two samples, which is what a
// caller measuring one frame actually wants.
func (c DeviceCounts) Sub(earlier DeviceCounts) DeviceCounts {
	return DeviceCounts{
		Textures:   c.Textures - earlier.Textures,
		Pipelines:  c.Pipelines - earlier.Pipelines,
		Buffers:    c.Buffers - earlier.Buffers,
		BindGroups: c.BindGroups - earlier.BindGroups,
	}
}

// Total is every kind together, for a caller that only wants "how much".
func (c DeviceCounts) Total() uint64 {
	return c.Textures + c.Pipelines + c.Buffers + c.BindGroups
}

// DeviceStats reports how many device resources have been created since the
// process started. Take a difference across a frame — see Sub — to see what
// that frame had to make.
func DeviceStats() DeviceCounts {
	return DeviceCounts{
		Textures:   texturesCreated.Load(),
		Pipelines:  pipelinesCreated.Load(),
		Buffers:    buffersCreated.Load(),
		BindGroups: bindGroupsCreated.Load(),
	}
}
