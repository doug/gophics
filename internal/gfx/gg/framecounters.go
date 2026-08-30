package gg

import "sync/atomic"

// Per-frame renderer counters: which tier drew what, and how much work the
// encoder did to draw it.
//
// These answer questions the existing device counters cannot. wgpu.DeviceStats
// says how many GPU objects a frame created; this says *which tier* was
// responsible, and whether the population of that tier explains the count.
// design/rendering-pipeline.md rests on the claim that tier 2b —
// stencil-then-cover, which has no anti-aliasing of its own — catches every
// stroke and every curved fill, and therefore most of a real UI. Nothing
// reported that, while the loop that decides whether a frame is empty summed
// exactly these seven populations and discarded the breakdown.
//
// They live in gg rather than in the gpu package because the gpu package is
// internal to gg: app can import gg and cannot import gg/internal/gpu. The
// counters are written by gpu and read by app with the dependency arrow
// unchanged, which is the shape RegisterCoverageFiller already uses.
//
// Process-wide atomics, reset at each frame boundary by the session that owns
// the frame. Correct for the single-window measurement corpus; a second window
// rendering concurrently would interleave, which is the same caveat
// wgpu.DeviceStats carries and for the same reason.
var (
	tierSDF        atomic.Uint64
	tierConvex     atomic.Uint64
	tierStencil    atomic.Uint64
	tierImage      atomic.Uint64
	tierGPUTexture atomic.Uint64
	tierText       atomic.Uint64
	tierGlyphMask  atomic.Uint64

	damageRefused atomic.Uint64
)

// FrameCounters is one frame's renderer work, by tier and by encoder activity.
type FrameCounters struct {
	// Tier populations: how many items each pipeline tier drew.
	SDF        uint64 // tier 1, analytic AA
	Convex     uint64 // tier 2a, per-vertex coverage ramp
	Stencil    uint64 // tier 2b, stencil-then-cover — the tier with no AA
	Image      uint64 // tier 3
	GPUTexture uint64 // tier 3b
	Text       uint64 // tier 4, MSDF
	GlyphMask  uint64 // tier 6, CPU alpha atlas

	// DamageRefused counts frames that computed a damage rect and threw it
	// away because the frame was not blit-only — the MSAA constraint at the
	// heart of F3b. "How many real frames trip it" is a number nobody had.
	DamageRefused uint64
}

// Total is every tier together: the number of drawable items in the frame.
func (f FrameCounters) Total() uint64 {
	return f.SDF + f.Convex + f.Stencil + f.Image + f.GPUTexture + f.Text + f.GlyphMask
}

// ReadFrameCounters returns the counters as they stand.
func ReadFrameCounters() FrameCounters {
	return FrameCounters{
		SDF: tierSDF.Load(), Convex: tierConvex.Load(), Stencil: tierStencil.Load(),
		Image: tierImage.Load(), GPUTexture: tierGPUTexture.Load(),
		Text: tierText.Load(), GlyphMask: tierGlyphMask.Load(),
		DamageRefused: damageRefused.Load(),
	}
}

// ResetFrameCounters zeroes them at a frame boundary.
func ResetFrameCounters() {
	for _, c := range []*atomic.Uint64{
		&tierSDF, &tierConvex, &tierStencil, &tierImage, &tierGPUTexture,
		&tierText, &tierGlyphMask, &damageRefused,
	} {
		c.Store(0)
	}
}

// CountTiers records one frame's tier populations. Called by the renderer at
// the point it already sums them to decide whether the frame is empty.
func CountTiers(sdf, convex, stencil, image, gpuTexture, text, glyphMask int) {
	add := func(c *atomic.Uint64, n int) {
		if n > 0 {
			c.Add(uint64(n)) //nolint:gosec // populations are small and non-negative
		}
	}
	add(&tierSDF, sdf)
	add(&tierConvex, convex)
	add(&tierStencil, stencil)
	add(&tierImage, image)
	add(&tierGPUTexture, gpuTexture)
	add(&tierText, text)
	add(&tierGlyphMask, glyphMask)
}

// encoderReset lets the frame boundary zero the encoder counters too without
// gg taking a dependency on wgpu. gg does not import wgpu anywhere — the gpu
// subpackage does — and adding that edge for a measurement would put a GPU
// binding in the dependency graph of every CPU-only build.
var encoderReset func()

// RegisterEncoderReset is called by the GPU layer during init.
func RegisterEncoderReset(fn func()) { encoderReset = fn }

func resetEncoderStats() {
	if encoderReset != nil {
		encoderReset()
	}
}

// CountDamageRefused records a frame that computed a damage rect and discarded
// it. Passes, draws and pipeline switches are counted in wgpu instead, at the
// encoder entry points — see wgpu/encoderstats.go for why the choke point is
// the right home for those and the tier populations above are not.
func CountDamageRefused() { damageRefused.Add(1) }
