package wgpu

import "sync/atomic"

// Per-frame transfer volume: bytes handed to the GPU, and bytes copied back.
//
// The counters already here answer "how many objects" and "how much encoder
// work". Neither answers "how much data moved", and that is the question a
// damage rect exists to change: damage-aware present asks for uploaded bytes
// to fall to the damage rect's area, and nothing counted bytes, so the work
// could not be shown to have worked. Frame time was tried as a
// proxy and is a bad one — a readback harness charges the full surface every
// frame whatever the damage says, which is enough to hide the effect entirely.
//
// Counted at the queue, which is the only way data reaches or leaves a device:
//
//   - BufferBytes  — WriteBuffer: vertices, indices, uniforms.
//   - TextureBytes — WriteTexture: glyph atlas pages, images, and the CPU
//     present path's whole surface.
//   - ReadbackBytes — MapAsync-backed reads, which is how an offscreen frame
//     comes back and the term that dominates the headless harness.
//
// Per-frame like EncoderStats and reset at the same boundary, because the
// interesting number is per frame rather than per process.
var (
	bufferBytes   atomic.Uint64
	textureBytes  atomic.Uint64
	readbackBytes atomic.Uint64
)

// TransferCounts is one frame's data movement, in bytes.
type TransferCounts struct {
	BufferBytes   uint64
	TextureBytes  uint64
	ReadbackBytes uint64
}

// Uploaded is everything sent to the device this frame.
func (t TransferCounts) Uploaded() uint64 { return t.BufferBytes + t.TextureBytes }

// TransferStats returns the counters as they stand.
func TransferStats() TransferCounts {
	return TransferCounts{
		BufferBytes:   bufferBytes.Load(),
		TextureBytes:  textureBytes.Load(),
		ReadbackBytes: readbackBytes.Load(),
	}
}

// ResetTransferStats zeroes them at a frame boundary.
func ResetTransferStats() {
	bufferBytes.Store(0)
	textureBytes.Store(0)
	readbackBytes.Store(0)
}

// countReadback records bytes read back from the device. Exported within the
// package for the map paths, which is where a readback actually lands.
func countReadback(n int) {
	if n > 0 {
		readbackBytes.Add(uint64(n)) //nolint:gosec // a slice length is non-negative
	}
}
