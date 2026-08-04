# Single-pass surface compositing — fixing MSAA multi-pass content loss

## The bug (confirmed on device)

On the mobile GPU present path (`encodeSubmitSurfaceGrouped` in gg's
`internal/gpu/render_session.go`), a frame that is flushed **more than once**
loses the content of the earlier flush on tile-based GPUs. Confirmed on a Pixel
10 Pro (Imagination PowerVR, Vulkan):

- Same scene (a red SDF circle + text), one difference — number of render passes:
  - **Single pass** (`LoadOpClear`): circle ✅ + text ✅
  - **Two flushes** (2nd pass = `LoadOpLoad`): circle ❌ **dropped**, text ✅

Despite `colorAttachment` using `StoreOp: StoreOpStore` (render_session.go ~430),
PowerVR does **not** preserve the multisampled attachment across a resolve +
reload. `render_session.go:913` already documents this ("MSAA render passes
cannot use LoadOpLoad because multisampled content is discarded after resolve"),
yet `encodeSubmitSurfaceGrouped` sets `colorLoadOp = LoadOpLoad` on every
continuation flush (frameRendered == true). It is **MSAA-attachment-reload in
general, not swapchain-specific** — the repro reproduced it resolving to a plain
offscreen texture, so "render offscreen then blit" does NOT sidestep it.

## Why it is latent today (but a real landmine)

A frame is only multi-pass when something forces a **mid-frame flush**:
`Context.flushGPUAccelerator()` runs before a CPU-fallback op so pending GPU
shapes land first. Triggers found (Explore of gg): **bitmap text**
(text.go:130/608/630/655), **FillRectCPU** (context.go:590), and **rotated /
non-axis-aligned DrawImage** (the pattern fallback in context_image.go). A frame
that stays entirely on the GPU tiers is single-pass and never hits the bug — which
is why gpucheck (all-GPU) renders correctly today. The moment a real app draws a
rotated sprite, a blurred/bitmap run, or any CPU-fallback content, the *next*
content in that frame lands in a `LoadOpLoad` MSAA pass and is corrupted on mobile.

Note a second, related gap surfaced in the same investigation: on the real GPU
strategy, CPU-fallback pixels written to `c.pixmap` are **never uploaded to the
surface at all** (only the base layer + GPU tiers composite). So CPU-fallback
content is *doubly* broken on the surface path: dropped on upload, and its flush
corrupts neighbours. The single-pass model below fixes both.

## The fix — one MSAA pass per frame

Never rely on MSAA `LoadOpLoad`. Two layers:

### A. Make multi-pass frames correct (accumulator + re-seed) — the safety net
- Add a persistent, **sampleable, single-sample accumulator** texture (ping-pong
  pair to avoid a read-after-write hazard) to `textureSet` (surface variant).
- Each surface flush resolves its MSAA pass into the *current* accumulator.
- On a **continuation** flush, first redraw the *previous* accumulator as a
  full-frame quad into the MSAA attachment under `LoadOpClear` (reuse the existing
  base-layer path — `imagePipeline` already draws a full-frame textured quad at the
  MSAA sample count), then draw the new content, then resolve. No MSAA reload ever.
- **Present-blit** the accumulator to the swapchain each flush using the existing
  single-sample `blitPipeline` (`blit.wgsl`, a 1:1 fullscreen triangle → LOD-0 safe).

### B. Reduce mid-frame flushes (Impeller-style) — the real win
- At each CPU-fallback site, instead of flush → draw-to-pixmap, **capture the
  fallback output as an ordered `QueueImageDraw`/`QueueGPUTextureDraw`** so the whole
  frame composites in ONE pass with `LoadOpClear`. This also fixes the
  "CPU-fallback never reaches the surface" gap.
- Ordering constraint (Explore finding): within a clip group, tiers render in a
  **fixed order** (SDF → images → text), regardless of submission order
  (`recordGroupDraws`). To place a captured fallback texture at the right z, force
  a **group boundary** at each insertion (groups render in submission order) — do
  not rewrite the tier model. The reserved `drawCommand.sortKey` is the alternative
  if per-draw ordering is ever needed.

With B in place, A rarely triggers; keep A as the correctness backstop for any
fallback not yet captured.

## The format landmine (must handle)

`ensureSurfaceTextures` hardcodes the MSAA color as **BGRA8Unorm**, and the
single-sample `blitPipeline` is built for a **BGRA8Unorm** target. But the Pixel
swapchain is **RGBA8Unorm** (`negotiateSurface` in shell/mobile/gpu.go picks it).
Today the code resolves the BGRA MSAA *directly* to the RGBA swapchain and it
happens to work; a present-blit **render pass** with a BGRA-format pipeline into an
RGBA swapchain view is a format mismatch and will fail validation. So:

- The present-blit pipeline must be created with the **actual swapchain format**,
  which the gg session does not track today (it hardcodes BGRA everywhere).
- Thread the surface format from `shell/mobile/gpu.go` (`negotiateSurface` result)
  through the accelerator/session so the accumulator + present-blit pipeline match
  it. This is the piece that intersects the in-flight `gpu.go` surface work — scope
  it there.

## Verification

- **Device repro (the gate):** the two-flush surface repro that dropped the circle
  must now keep both circle + text. Build a small harness that renders shapes,
  forces a mid-frame flush, then renders text, presents, reads back.
- **Parity:** `app/gpu_equiv_test.go` (`TestGPUMatchesCPU`) and the render-matrix
  gate must stay green — the accumulator/blit must not change single-pass output.
- **On-device lifecycle:** run the bg/fg + rotation stress (now that the atlas
  device-recreation bug is fixed) and a scene that actually draws a rotated sprite
  (forces the fallback → the multi-pass path) to confirm no corruption.

## Risk

This is on the shared present critical path (desktop/web/mobile) and touches
format/lifecycle code adjacent to in-flight `shell/mobile/gpu.go`. A wrong
composite or a format mismatch corrupts every frame on every platform. Land behind
the device repro + parity gates, validate on the Pixel, and thread the swapchain
format explicitly rather than assuming BGRA.
