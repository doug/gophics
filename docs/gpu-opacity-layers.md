# GPU opacity-layer compositing — design & plan

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
