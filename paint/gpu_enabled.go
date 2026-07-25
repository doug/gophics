//go:build gossamer_gpu

package paint

// Building with -tags gossamer_gpu registers gg's GPU accelerator: SDF
// shapes, tiled path rasterization, and MSDF text via wgpu compute.
//
// EXPERIMENTAL — spike verdict (2026-07-25, gg v0.50.x): registration is
// process-global and the GPU path implements no CPU readback, so offscreen
// rendering (golden tests, Headless, the web PixelTarget) produces BLANK
// images under this tag. Do not use it for anything headless. On-screen
// rendering is unvalidated pixel-wise (no readback to compare). Adoption is
// blocked on gg providing per-context opt-in and offscreen readback; see
// PLAN.md §5.1. Default builds stay on the deterministic CPU rasterizer.
import _ "github.com/gogpu/gg/gpu"
