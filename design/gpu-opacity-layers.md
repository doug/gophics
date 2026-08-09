# GPU opacity-layer compositing — design & plan

> **STATUS 2026-08-04 — IMPLEMENTED & VERIFIED headless + on-device (uncommitted
> WIP in `internal/gfx/gg`).** On-device: an opacity region added to
> `examples/gpucheck` renders correctly on a real Pixel (PowerVR D-Series Vulkan,
> `RGBA8Unorm`, GPU present) — base survives, overlay 50%, nested 25%, matching
> the desktop reference. Deploy: `gophics run -p android -tags gophics_verify
> ./examples/hn/mobile` (needs the `replace` directives in `gophics/go.mod` —
> gomobile ignores `go.work`). Route B landed: opacity groups render
> to pooled offscreen targets via child contexts and composite with the group
> alpha (Skia saveLayer). New driver `gg/internal/gpu/gpu_layers.go`; hook at the
> top of `GPURenderContext.Flush`; `Context.PushLayer/PopLayer` branch to the GPU
> path in `context_layer.go`. Verified on headless Metal: `TestGPUOpacityGroup`
> (base survives + correct 50% composite; whole-frame GPU==CPU **0.000%** at
> 1×/2×/3×, overlay to the bottom-right corner guarding the HiDPI class) and
> `TestGPUOpacityNested` (child-of-child, 0.000%), in `app/gpu_equiv_test.go`.
> Regression guards green; the 3 `TestMetalStencil*` failures are pre-existing
> (software-backend MSAA, unrelated). **Remaining:** on-device Pixel-10 GPU-present
> validation (RenderPass mode). Deferred: per-draw blend modes (only `BlendNormal`
> reachable today), bounds-sized targets (perf), CPU-fallback-op-inside-a-layer.
> Implementation notes below reflect what was built; original plan follows.

`paint.PushOpacity`/`PopOpacity` (and `widget.Opacity`) don't work on the GPU
render path: content drawn *before* a group is lost and the group's alpha is
ignored (it renders at full opacity). The CPU path is correct. This is a
**missing feature**, not a localized bug — GPU-native layer compositing was never
implemented. This doc is the plan to add it.

Surfaced by the render-correctness harness (`internal/renderref`,
`app/rendermatrix_test.go`); see the `opacity-accelerator-bug` /
`gpu-backend-gaps` notes. Now that GPU present is verified on device (Pixel 10,
see `mobile-gpu-bringup.md`), opacity groups on real screens render wrong.

## Why it's broken (root cause)

`gg.Context` renders through a **hybrid** model: a CPU `pixmap` plus an optional
GPU accelerator (`SDFAccelerator` for circles/rrects, `AdaptiveFiller` coverage
for paths) and pipeline modes (Compute / RenderPass / Auto).

`Context.PushLayer` (`context_layer.go`) implements a layer as a **new CPU
pixmap** that it swaps `c.pixmap` to, compositing it back on `PopLayer`
(`compositeLayer`). This is correct for the CPU-only path (verified: 128
sequential layers render fine). But when the accelerator is active, fills are
rendered on the **GPU**, not into `c.pixmap` — so:

- content drawn before `PushLayer` lives in the GPU target, not the (now-swapped)
  base pixmap → it's lost when the empty base pixmap is composited;
- the layer's fills also go to the GPU target, not the layer pixmap → no
  isolation, so alpha can't be applied at composite time.

Confirmed dead ends (don't retry these):

- `internal/gpu` `GPUSceneRenderer.pushLayer` (has `AllocTexture` + a
  `// TODO: get actual alpha` `blendTextures(..., 1.0)`) is **never reached** in
  `RenderGPU` — dead/alternate code.
- `gg/scene/gpu_renderer.go` decodes `TagPushLayer` straight back to
  `dc.PushLayer` — i.e. the CPU-pixmap path again.
- Forcing `PipelineModeRenderPass` (what mobile/web use) makes it **worse**
  (loses base content *and* still ignores alpha).

## The target model (what to build)

The standard approach (Skia/Flutter/Vello): a layer is an **offscreen GPU render
target**. `PushLayer` allocates a texture, redirects rendering to it; `PopLayer`
composites it onto the parent target with the layer's alpha + blend mode, then
frees it.

    pushLayer(blend, alpha):
        tex = allocRenderTarget(bounds)   # physical/device pixels — see note
        clear(tex, transparent)
        stack.push({target: tex, blend, alpha, parent: currentTarget})
        currentTarget = tex               # subsequent fills render here

    popLayer():
        l = stack.pop()
        currentTarget = l.parent
        drawTexturedQuad(dst=l.parent, src=l.target, alpha=l.alpha, blend=l.blend)
        freeRenderTarget(l.target)

Key correctness points (each maps to a bug we already hit):

1. **Alpha must be threaded through** to the composite (the `1.0` TODO). The
   layer's `alpha` multiplies at the composite quad, not per-fill.
2. **The parent target is preserved** — the layer composites *over* it
   (source-over), it does not replace it. (This is the "blue rect lost" bug.)
3. **Size the offscreen target in PHYSICAL pixels** (`bounds × deviceScale`), not
   logical — the exact class as the CPU HiDPI `PushLayer` bug (fixed) and the
   headless-scale bug (fixed, `a618e37`). Prefer sizing to the layer's content∩clip
   `Bounds` (already tracked on `scene.LayerState.Bounds`) to keep it cheap;
   full-surface targets are the simple first cut.
4. **Nesting**: `currentTarget` is a stack; a nested push targets the enclosing
   layer's texture, not the base. Clip and transform state must be saved/restored
   across push/pop (the encoding already tracks `ClipStackDepth`/`Transform` on
   `LayerState`).

## Where it hooks in

`Context.PushLayer`/`PopLayer` must branch: CPU-pixmap path when no GPU
accelerator; GPU offscreen-target path when one is active. The GPU path needs the
accelerator to expose render-target push/pop+composite. Two ways to get there:

- **Revive + wire `GPUSceneRenderer`** (`internal/gpu/renderer.go`): it already
  has `pushLayer`/`popLayer` with `AllocTexture`/`blendTextures`/`FreeTexture` —
  fix the alpha and base-preservation, then connect it to the active `RenderGPU`
  path (understand *why* it's currently bypassed).
- **Implement in the active accelerator path** (`SDFAccelerator` + coverage
  filler + `gogpu`): add render-to-texture target push/pop there. `gogpu` already
  renders to offscreen textures (`RenderToImage`), so the primitive exists.

Which of these is right depends on **which renderer is canonical** — that's the
main open question below.

## Open questions (need architectural context before coding)

The GPU stack has several partial/overlapping renderers; pick the canonical one
first to avoid building on dead code:

1. **Canonical GPU renderer?** `SDFAccelerator`+filler+pixmap vs
   `internal/gpu.GPUSceneRenderer` vs `scene/gpu_renderer.go` vs `gogpu` direct.
   `RenderGPU`, the desktop present, and the mobile `RenderDirect` present may
   not all use the same one — layers must be added to the one(s) that ship.
2. **Pipeline mode.** Compute (CPU-pixmap readback) vs RenderPass. Mobile/web
   force RenderPass; headless sets none. Layers likely belong in the RenderPass
   path — confirm and make headless match so the parity test exercises the real
   path.
3. **Target-management API.** Does the accelerator interface need a new
   `PushRenderTarget/PopRenderTarget` (analogous to `PipelineModeAware`), or does
   it go through `gogpu` render targets directly?

## Verification

`internal/renderref` is already built for this. When GPU layers work:

- Flip the GPU parity test to the **full** `renderref.Scene()` (it includes a
  grid of opacity groups reaching the bottom-right corner) across **1×/2×/3×**,
  asserting GPU==CPU within the AA tolerance (~7% at HiDPI, per the primitives
  gate). Split `Scene()` into `ScenePrimitives()` + opacity if a staged gate
  helps.
- Add a focused unit check: base fill + `PushOpacity(0.5)` + overlapping fill →
  the base survives and the overlap is ~50%. (This is the minimal repro that
  currently fails: base lost, overlap full-opacity.)
- Re-run the CPU scale-consistency gate (must stay green) and `TestGPUMatchesCPU`
  (single-scale primitives) as regression guards.

## Risk

This is on the GPU-present critical path (desktop/web/mobile). A wrong composite
or target-lifecycle bug corrupts every frame on every platform. Land behind the
harness above, and validate on device (Pixel 10 GPU present) — not just headless
`RenderGPU` — before calling it done, since the two paths have already diverged
once (the headless-scale bug that never affected the device).

---

## RESOLVED (2026-08-04): canonical path, chosen route, implementation plan

Two deep code traces (canonical-renderer + offscreen-primitive catalog) settled
every open question above. Summary of what's now known and the plan.

### Canonical renderer (Open Q1 — answered)

**All three present paths converge on ONE renderer.** Desktop
(`shell/desktop/present.go` → `ggc.Render` → `RenderDirect`), web
(`shell/web/present.go` → `RenderDirect`), and mobile (`shell/mobile/gpu.go` →
`RenderDirect`) all funnel through `ggcanvas.Canvas.Draw` +
`gg.Context.FlushGPUWithView` → the **`SDFAccelerator`'s per-`gg.Context`
`*GPURenderContext`** (`internal/gpu/gpu_render_context.go`), whose `Flush`
(`:850`) dispatches a **`GPURenderSession`**. gophics replays its own display
list into `gg.Context` method calls (it does *not* use gg's `scene` package);
`paint.PushOpacity`→`Context.PushLayer(BlendNormal, alpha)` (`paint/paint.go:919`).

Confirmed dead (do NOT build on): `internal/gpu/renderer.go`
`GPUSceneRenderer.pushLayer`/`blendTextures` (only test/example callers; its
`CreateTexture` is a stub that allocates no GPU memory), and
`scene/gpu_renderer.go` (gophics never imports `gg/scene`).

### Chosen route: B — layers in the active accelerator path

Route A (revive `GPUSceneRenderer`) is a from-scratch build on stubs. **Route B
reuses machinery that already works**: real offscreen MSAA+resolve target sets
(`textureSet.ensureTextures`, `gpu_textures.go:38`), a dormant pool built for
exactly this (`TexturePool.Acquire/Release/EndFrame`, `texture_pool.go:57`,
Flutter `RenderTargetCache` pattern — currently never called outside tests),
retargetable passes (`GPURenderTarget.View` + `resolveActiveView` +
`effectiveDimensions`), a no-submit shared encoder (`encodeToEncoder`,
`render_session.go:3277`, ADR-017 "Impeller pattern"), and a **working
GPU-texture→target compositor with per-draw opacity**
(`buildGPUTextureResources` + `QueueGPUTextureDraw` +
`GPUTextureDrawCommand.Opacity`).

### The load-bearing constraint (drives the whole shape)

The frame is **ONE render pass** (`LoadOpClear` once; verified on-device:
1053/1053 single-pass, scissor *groups* are scissor rects within the one pass,
not separate passes). Within that pass, draws are **bucketed by primitive type**
(`SDFShapes`, `ConvexCommands`, `StencilPaths`, `ImageCommands`, `TextBatches`,
`GlyphMaskBatches`, `GPUTextureCommands`) and dispatched in fixed **tier order** —
so z-order within a clip group is tier order, *not* insertion order
(`drawsToScissorGroup`, `gpu_render_context.go:1042`). This is a pre-existing
property gophics's GPU renderer already lives with.

Design consequence: **a layer renders to its own offscreen texture in a separate
prior pass (`LoadOpClear`, TBDR-safe — never `LoadOpLoad`), and its resolved
single-sample texture is composited into the single main pass as a
`GPUTexture`-tier quad with the group's alpha.** The layer's internal sample
count need not match the parent; only its 1× resolve output is sampled. This is
exactly Skia/Impeller `saveLayer`.

Known z-order limitation (inherited, documented): content of a *different
primitive type* drawn after an opacity group and *overlapping* it at the *same
clip* composites in tier order, not strict painter's order — same class as the
renderer's existing mixed-type ordering limitation. Opacity groups typically wrap
a leaf subtree with non-overlapping / differently-clipped siblings, so this is
rare in practice; the `renderref` opacity grid validates the common cases.

### Scope decisions

- **Full-surface layer targets first cut** — mirror the CPU `PushLayer` (sizes to
  `c.pixmap` physical dims, not logical). Composite quad is a full-surface quad at
  origin; layer draws use absolute coords. Bounds-sized targets (cheaper) are a
  later optimization using `scene.LayerState.Bounds`.
- **Per-draw alpha only; DEFER blend modes.** `PushOpacity` always uses
  `BlendNormal`+alpha, and the composite pipeline is hardcoded
  `BlendStatePremultiplied` (premultiplied source-over) — which is exactly right
  for opacity groups. Separable/HSL blend modes (Multiply/Screen/…) would need a
  dst-sampling shader + pipeline; not reachable from gophics today, so out of
  scope for this landing.
- **Clip/transform save-restore** is largely already handled: gg bakes clip state
  into each `drawCommand` at record time (`clipRect`/`clipRRect`/`clipPath`,
  `gpu_render_context.go:837`) and transform into shape coords, so a layer's draws
  carry their own state. No global save/restore stack needed for the first cut.

### Build plan (Route B) — refined to the multi-context / RepaintBoundary pattern

Key refinement found while reading the session: **a layer is a RepaintBoundary
with an opacity composite.** Rendering a layer's draws into an offscreen
single-sample sampleable color target can reuse `RenderFrameGrouped` *wholesale*
(the shared MSAA/stencil scratch `s.textures` is safe to reuse across sequential
`LoadOpClear` passes — only the resolve target differs). **Hazard:**
`RenderFrameGrouped` writes into session-level shared GPU buffers
(`render_session.go:701-705`), so calling it N times per submit on one encoder
would let the last write clobber all passes. **Resolution:** use **one
`GPURenderContext`/session per layer**, all recording into the ADR-017 shared
encoder — exactly the multi-context / RepaintBoundary path gg already supports
(`warnGPUFallback` references it; multiple `gg.Context`s already share one encoder
today). So no `RenderFrameGrouped` rewrite and no texture-set stack.

1. **`LayerAware` per-context ops** — add `PushLayer(opacity float64, blend
   BlendMode)` / `PopLayer()` to `GPURenderContext`; detect from `Context` via an
   inline interface on `gpuCtxOps()` (the `SetSharedEncoder`/`CreateEncoder`
   pattern at `context.go:1335`). Layers are per-context state, so this goes
   through the per-context rc, not the global accelerator.
2. **Branch `Context.PushLayer`/`PopLayer`** (`context_layer.go`): when
   `rc := c.gpuCtxOps()` is present and GPU is the live path, record a layer
   marker and SKIP the CPU-pixmap swap (the swap is what breaks the GPU path);
   keep the CPU-pixmap path unchanged when `rc == nil`.
3. **Record push/pop markers in `pendingDraws`** (`gpu_render_context.go`): new
   `drawCmdPushLayer`/`drawCmdPopLayer` kinds carrying `{opacity, blend}`.
4. **Layer-aware Flush driver**: at end-of-frame, partition `pendingDraws` into a
   nesting tree by push/pop. Depth-first, innermost layers first: acquire a
   transient sampleable color target (WxH, `RenderAttachment|TextureBinding|
   CopySrc`), render that layer's own draw run through a pooled child
   `GPURenderContext`/session via `FlushGPUWithView(layerColorView, W, H)` on the
   shared encoder (view change → auto `LoadOpClear`, TBDR-safe), then record
   `GPUTextureDrawCommand{View: layerColorView, Opacity: alpha, Dst: 0,0,W,H}`
   into the parent stream at the pop position. Parent stream flushes last into the
   surface, compositing the layer quads in the single main pass. Sequential passes
   on one encoder provide the read-after-write dependency (parent samples the
   layer's resolve output).
5. **Transient layer-target pool + child-session pool**: a size-keyed pool of
   single-sample sampleable color textures (new — `textureSet` resolve is
   `CopySrc`-only, not `TextureBinding`), plus a small pool of child
   `GPURenderContext`s so per-layer buffers are independent; recycle at frame end.
   (`TexturePool` pools full MSAA sets; the new pool is just the sampleable color
   target.)

Open impl risks to confirm while coding: (a) child sessions must share
`GPUShared` pipelines/atlases (read-only) but own independent vertex/index/uniform
buffers — verify `NewGPURenderSession` gives per-session buffers; (b) a CPU-
fallback op *inside* a layer (e.g. gradient) must flush into that layer's target,
not the parent — the layer's own context/target handles this since fallback
flushes go through the context's `gpuRenderTarget()`; confirm the layer context's
target is the layer view; (c) glyph/MSDF atlas views are engine-shared — ensure
child sessions get `SetGlyphMaskAtlasView`/`syncTextAtlases` like the parent.

### Verification (Task 4)

- Focused unit: base fill + `PushOpacity(0.5)` + overlapping fill → base survives,
  overlap ~50% (the minimal repro that fails today).
- Flip `app/rendermatrix_test.go` GPU parity to the full `renderref.Scene()`
  (opacity grid) at 1×/2×/3× within AA tolerance.
- Regression guards stay green: CPU scale-consistency + `TestGPUMatchesCPU`.
- Reconcile desktop (Auto pipeline mode) vs web/mobile (forced RenderPass) so the
  layer path is exercised identically; make headless match the shipping path.
- On-device validation on the Pixel 10 GPU present — not just headless `RenderGPU`.
