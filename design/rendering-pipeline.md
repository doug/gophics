# Rendering pipeline — improvement and cleanup plan

> **STATUS 2026-08-29 (rev 4) — Phase A partly landed; F1 and F2 confirmed,
> §1.2 corrected.**
>
> | Phase | Status | Number it moved | Measured on |
> | --- | --- | --- | --- |
> | A1 buffer/bind-group counters | **done** | F1 made observable | Metal, M-series |
> | A2 tier populations | **done** | §1.2 corrected — see below | Metal |
> | A3 passes / draws / switches | **done** | F2 confirmed | Metal |
> | A4 MSAA ablation hook | **done** | MSAA priced: free on Metal | Metal, M-series |
> | C1 hoist tier 2b allocations | **done** | 121→1 obj/frame; -45% frame time | Metal, M-series |
> | glyph bind-group reuse (unplanned) | **done** | 38→0 bind groups/frame; no time change | Metal, M-series |
> | C2 strokes → tier 2a | **reverted** | 2b 75%→15%, but parity 0.48%→10.27% | Metal, M-series |
> | F7 text layout divergence | **fixed** | GPU drift 9px→0; text parity 11.0%→4.98% | Metal + **Vulkan/Mali** |
> | opacity single-draw fold | **done** | 21→1 passes; Mali 53.7→11.6 ms (4.4×) | Metal + **Vulkan/Mali** |
> | D coverage-AA instead of MSAA | **closed, not justified** | MSAA now free on both backends | Metal + **Vulkan/Mali** |
> | Vulkan verification | **done** | MSAA = 2× on multi-pass; C1/F7 hold | **Mali-G615, Galaxy Tab A9+** |
> | A5 corpus + baseline file | scenes landed; baseline file not started | — | — |
> | A6 Metal timestamp honesty | **done** | one lie removed | Metal |
>
> **F1 is real, and the ratio is exact.** Steady state, after warm-up, zero
> pipelines created — so this is per-frame churn, not first-use cost. Measured
> against `createRenderBuffers`' 4 buffers + 2 bind groups per path:
>
> | Scene | 2b paths | predicted | measured per frame |
> | --- | --- | --- | --- |
> | curve-heavy | 24 | 96 buf / 48 bg | **97 / 48** |
> | stroke-heavy | 30 | 120 / 60 | **121 / 60** |
> | ui-screen | 12 | 48 / 24 | **49 / 62** |
> | text-heavy | 0 | ~0 | **1 / 2** |
>
> **F2 is real.** Pipeline switches ≈ 2× the stencil population: 49 measured
> against 48 predicted on curve-heavy, 62 against 60 on stroke-heavy.
>
> **§1.2 is overstated and is corrected below.** "Tier 2b catches most of a UI"
> holds for stroke- and curve-dominated content (75%, 92% of items) but not for
> a real screen: `renderref.UIScreen()`, six themed cards with buttons, text and
> dividers, is **23%** tier 2b — 12 paths of 51 items, behind 20 SDF and 19
> glyph batches. §4.6 set this as the kill condition for Phases C and D, so it
> matters that it is not met: 23% is not a small minority, and those 12 paths
> cost **111 GPU objects created and destroyed every frame**. Phase C stands;
> the sentence that motivated it does not.
>
> One detail worth chasing: on ui-screen, pipeline switches (62) *exceed* draws
> (43), which the stencil alternation alone does not explain.
>
> **The MSAA ablation says Phase D is not justified — on Metal.** `GOGPU_NO_MSAA=1`
> against the default, mean of 40 frames after warm-up, repeated three times
> because the first run's ui-screen figure was a cold-machine outlier that
> flattered the ablation by 42%:
>
> | Scene | 4× MSAA | 1× ablated |
> | --- | --- | --- |
> | mixed | 4.94 / 4.92 ms | 4.91 / 5.03 ms |
> | ui-screen | 2.29 / 2.36 ms | 2.33 / 2.32 ms |
> | stroke-heavy | 3.09 ms | 3.14 ms |
> | curve-heavy | 2.38 ms | 2.41 ms |
>
> No difference on any scene. §3's bandwidth argument does not hold on an
> M-series, which has bandwidth to spare — and §6 Phase D is explicitly gated on
> this: "if 4× MSAA is cheap on both backends this phase is not worth its risk."
>
> **The tiler half is now measured, and it justifies Phase D — with a sharper
> mechanism than §3 proposed.** Mali-G615 MC2, Vulkan, Galaxy Tab A9+, three
> repeats each:
>
> | Scene | passes | 4× MSAA | 1× ablated |
> | --- | ---: | --- | --- |
> | mixed | **21** | 53.7 / 54.3 / 55.8 ms | **27.1 / 27.7 / 27.7 ms** |
> | ui-screen | 1 | 10.2 / 11.6 / 12.5 ms | 12.7 / 10.6 / 12.1 ms |
> | stroke-heavy | 1 | 9.6 ms | 9.3 ms |
> | curve-heavy | 1 | 8.8 ms | 7.8 ms |
> | text-heavy | 1 | 9.3 ms | 10.1 ms |
>
> MSAA costs **2× on the multi-pass scene and nothing measurable on any
> single-pass scene**. The distinguishing variable is not content — stroke-heavy
> and curve-heavy are the tier-2b-dominated scenes and they do not move — it is
> the render pass count. §3 predicted "4× the attachment and a resolve every
> pass"; the measurement says the per-pass resolve is the whole term, at roughly
> 1.3 ms per pass on this device.
>
> **What spends the passes: opacity groups, one pass each.** Measured by adding
> one construct at a time to a fixed baseline (`TestWhatCostsARenderPass`):
>
> | Scene | passes |
> | --- | ---: |
> | baseline, one fill | 1 |
> | + rect clip | 1 |
> | + rrect clip | 1 |
> | + nested clips | 1 |
> | + gradient | 1 |
> | + sprite | 1 |
> | **+ opacity group** | **2** |
> | **+ two opacity groups** | **3** |
>
> Clips, gradients and sprites are free; every opacity group costs a pass. That
> accounts for the reference scene exactly: its tile loop is 2 rows × 5 columns
> with two PushOpacity calls per tile — 20 groups, plus the base pass, is the 21
> observed. At ~1.3 ms per pass on the Mali, `1 + 20 × 1.3` is the 53 ms frame.
>
> The scene's own comment says those tiles carry two layer pushes "like real
> UIs", and that is the part which generalizes: a UI fading ten elements pays
> ~13 ms/frame on this class of device for the layers alone, which drops a frame
> by itself. `ui-screen` has no opacity groups and runs 1 pass, which is why it
> was never slow.
>
> **Fixed by folding single-draw groups.** A group holding exactly one draw does
> not need an offscreen at all: source-over of one primitive is associative in
> alpha, so drawing it into a transparent layer and compositing at group alpha g
> equals drawing it at colour-alpha × g. `GPURenderContext.PopLayer` now
> collapses push/draw/pop into the draw, guarded on all three conditions that
> make the identity hold — exactly one draw (two would double-blend where they
> overlap), Normal blend (any other is defined against a backdrop, and a layer's
> is transparent), and a solid colour (a gradient's alpha is per-stop).
>
> The reference scene's 20 groups are all single-draw, so:
>
> | | before | after |
> | --- | --- | --- |
> | passes | 21 | **1** |
> | draws | 58 | 38 |
> | Mali frame, mean of 3 | 53.7 / 54.3 / 55.8 ms | **11.6 / 12.8 / 12.2 ms** |
> | Mali frame, best | 42.3 ms | **4.0 ms** |
>
> **4.4× on the tiler**, and pixel-identical: GPU/CPU parity on `mixed` is
> 1.178% before and after. That is the right check rather than a weak one — the
> CPU path does not fold, so the GPU agreeing with it exactly as much as it did
> before is evidence the fold changed nothing visible.
>
> `TestWhatCostsARenderPass` gates both directions: a single-draw group must
> cost 1 pass, a two-draw group must cost 2. The second matters as much as the
> first, because folding it would be silently wrong rather than slow.
>
> This did not need the offscreen batching `design/gpu-opacity-layers.md`
> scopes. Groups with several draws still take a pass each, and batching
> non-overlapping ones remains available if a real UI is ever found that needs
> it — but the common shape, a fade over one thing, now costs nothing.
>
> **And that closes Phase D.** Re-running the ablation after the fold, three
> pairs on the same Mali:
>
> | Scene | 4× MSAA | 1× ablated |
> | --- | --- | --- |
> | mixed | 13.7 / 10.3 / 10.8 ms | 11.9 / 11.7 / 11.3 ms |
> | stroke-heavy | 12.0 / 11.6 / 11.3 ms | 10.8 / 11.9 / 10.7 ms |
>
> No consistent difference, and 1× is often slower — the signature of noise
> rather than a small win. The 2× that MSAA cost before the fold was
> `20 passes × per-pass resolve` and nothing else.
>
> So the phase this plan called its largest item, and built §3's whole root-cause
> chain around, is **not justified on either backend**. Its premise was that 4×
> MSAA taxes every tier to anti-alias tier 2b. The tax was real and was a
> property of the *pass count*, not of the tier, and the pass count was fixed by
> a forty-line fold with no quality cost — where Phase D proposed a
> coverage-rasterizer rewrite that would have changed edge quality by
> construction.
>
> What survives from §3: F3a's multi-pass corruption is still real and still
> wants `design/gpu-single-pass-surface.md`, and damage-rect scissoring on vector
> frames (F3b) is still refused by MSAA. Neither needs Phase D — the first is an
> accumulator, and the second is worth re-measuring now that a UI frame is one
> pass.
>
> **The old reasoning, kept because it was wrong in an instructive way.** The cost is `passes × MSAA-per-pass`, so there are
> two factors to attack and the plan only considered one. Removing MSAA is the
> larger, riskier change and costs edge quality; reducing the pass count helps
> the same frames by the same mechanism, costs no quality, and is what
> `design/gpu-single-pass-surface.md` already scopes for F3a's corruption. A
> scene needing 21 passes is itself the anomaly — and 53 ms/frame on a mid-range
> tablet is a real problem whichever factor is attacked.
>
> Everything else was backend-independent: tier populations, encoder counts,
> post-C1 churn (0 bind groups), corpus parity within 1–3 px of Metal, and the
> F7 text metrics identical to the digit.
>
> **How this was measured**, because it is cheaper than it looks and nothing
> recorded it:
>
>     CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -tags gophics_gpu -o app.test.android ./app
>     adb push app.test.android /data/local/tmp/ && adb shell chmod +x /data/local/tmp/app.test.android
>     adb shell "cd /data/local/tmp && ./app.test.android -test.run TestMSAAAblation -test.v"
>
> The zero-CGo build means the GPU test binary cross-compiles and runs under
> `adb shell` with no app, no JNI and no gomobile — headless Vulkan comes up
> against `/vendor/lib64/hw/vulkan.mali.so` directly. Prefix `GOGPU_NO_MSAA=1`
> for the ablation.
>
> Phase C is unaffected: it is justified by object churn, not by MSAA.
>
> **C1 landed and hit its target.** Tier 2b now creates nothing per frame in
> steady state. Five of the six objects turned out to need no capacity tracking
> at all — the cover quad and both uniforms are fixed-size, and the bind groups
> only reference those uniform buffers — so only the fan vertices needed the
> grow-only treatment buildConvexResources already used one function away.
>
> | Scene | objects/frame before | after | frame time before → after |
> | --- | --- | --- | --- |
> | stroke-heavy | 121 buf / 60 bg | **1 / 0** | 3.09 → 1.71 ms (**-45%**) |
> | curve-heavy | 97 / 48 | **1 / 0** | 2.38 → 1.55 ms (**-35%**) |
> | ui-screen | 49 / 62 | **1 / 38** | 2.33 → 1.83 ms (**-21%**) |
> | mixed | 13 / 34 | **1 / 28** | 4.93 → 4.82 ms (-2%) |
> | text-heavy | 1 / 2 | 1 / 2 | 1.34 → 1.31 ms (unchanged) |
>
> Three repeats each; text-heavy not moving is the control, since it had no
> churn to remove. GPU/CPU parity and scale consistency are identical to the
> pixel before and after — 1845/132000 differing, worst diff 161 at the same
> cell — so this is a pure allocation change.
>
> **It also exposed the next one, which is now fixed.** ui-screen was still
> making 38 bind groups a frame and none were tier 2b. Not buildTextResources,
> which is already well-behaved — the glyph-mask tier:
> `materializeGlyphMaskBindGroups` released and recreated every batch's bind
> group unconditionally, every frame. Keying each bind group on what it actually
> references — atlas view, layout, uniform buffer, sampler — makes an unchanged
> batch reuse it. The layout is in the key deliberately: BUG-GPU-001 was a bind
> group outliving the layout it was built against, and keying on it turns that
> into a rebuild instead of a stale binding.
>
> ui-screen and text-heavy now allocate **nothing** per frame: 1 buffer, 0 bind
> groups. **But it bought no time on Metal** — 1.83 → 1.85 ms, inside the noise.
> Bind-group creation is cheap there. §2.1 predicts it is not on Vulkan, where
> every set goes through descriptorAllocator.Allocate/Free per path per frame,
> and that is unmeasured. Recorded as churn removed, not as a speedup.
>
> What is left, and is a third subsystem again: mixed still makes 26 bind groups
> and 17 textures a frame, from the image/sprite and offscreen-layer paths.
>
> **C2 was implemented, measured, and reverted.** Removing the EvenOdd gate does
> what §6 C2 predicts — 24 of stroke-heavy's 30 stencil paths move to the convex
> tier, dropping 2b from 75% to 15% of items — and the fill-rule argument holds:
> on a verified convex simple polygon EvenOdd and NonZero classify every point
> identically, so the routing is not incorrect.
>
> But it renders worse. GPU/CPU agreement on stroke-heavy goes from **0.48% of
> pixels differing to 10.27%**, a 21× regression, while every other scene is
> untouched. The unstated assumption in C2 was that the two tiers render
> equally; they do not. Tier 2a's per-vertex coverage ramp is a cruder
> approximation for a thin stroke outline than 4× MSAA'd stencil, which is the
> same asymmetry §1.1 records in the "Own AA?" column read the other way round:
> 2b has no AA of its own *and gets the good one from MSAA*.
>
> So C2 is blocked on tier 2a's AA quality, not on its routing. It becomes
> available if Phase D gives 2b coverage-based AA and drops MSAA — at which
> point 2a and 2b would be compared on equal terms, and this measurement should
> be redone rather than assumed.
>
> **The corpus was not under any correctness test until this was written**, which
> is how C2 nearly landed. `TestGPUMatchesCPU` renders its own `equivScene` and
> `TestRenderScaleConsistency` renders `renderref.Scene`; neither ever saw
> StrokeHeavy or the others, so the parity number was identical before and after
> a change that moved 24 paths to a different rasterizer. A5's claim that
> putting the scenes in renderref would let the harnesses pick them up for free
> was wrong. `TestGPUMatchesCPUOnCorpus` closes it, with per-scene budgets.
>
> **The finding it surfaced, now diagnosed — F7, and it is a correctness bug
> rather than a performance one.**
>
> text-heavy's 11.0% is not anti-aliasing noise. The backends lay the same
> string out differently: every line *starts* on the same pixel and *ends* on a
> different one, drifting up to **9 pixels across 43 characters at 9px**, with
> total ink within 0.2% and no global shift that improves agreement. That is
> accumulation, not offset.
>
> `glyph_mask_engine.snapXGrid` (`:262`) gives each GPU glyph a position
> accumulated from **rounded** advances — a deliberate hinted-text technique,
> documented there, that keeps vertical stems on pixel boundaries and is the
> right call for crispness taken on its own. `text/glyph_renderer.go:251` places
> CPU glyphs at the shaper's exact `glyph.X`. `text/layout.go:495`
> (`MeasureWidthIn`) also sums unrounded advances.
>
> So **measurement and the CPU agree; the GPU disagrees with both** — and the
> GPU is the default renderer. Measured against MeasureWidthIn on a 43-character
> string:
>
> | size | measured | CPU ink Δ | GPU ink Δ | GPU − CPU |
> | --- | --- | --- | --- | --- |
> | 9 | 178.95 | −2.0 | **+7.0** | **+9.0** |
> | 11 | 218.98 | −3.0 | +0.0 | +3.0 |
> | 15 | 298.47 | −2.5 | **−5.5** | −3.0 |
>
> The CPU's constant −1.7 to −3.0 is side bearings — ink is narrower than
> advance width, as it should be. The GPU's swing from +7 to −5.5, changing sign
> with size, is the accumulated rounding.
>
> What that costs, since layout is computed from the measured width: centred
> text is off-centre, text sized to its measured box can overflow it, and a
> caret positioned from shaper metrics does not sit where the glyph was drawn —
> all on the GPU backend only, and worst at the small sizes UIs use most.
>
> **Fixed by removing snapXGrid.** The shaper looked like the right home for a
> single shared answer, but snapping is not a property of the text: it is a
> function of device scale, hinting mode, LCD mode and the transform, none of
> which the shaper knows and all of which belong to the target. LCD deliberately
> keeps fractional X because the fraction selects the R/G/B phase, so a shaper
> that snapped unconditionally would break subpixel rendering outright.
>
> What made the choice clear is that snapXGrid was the only snapping in the
> codebase that *accumulated*. The autohinter rounds each advance in f26dot6 and
> applyGridFit snaps outline coordinates inside a glyph; both are bounded per
> glyph. Only the pen-position grid compounded, which is why the error grew with
> string length instead of staying within half a pixel.
>
> Removing it makes GPU ink match CPU ink exactly at four of five sizes and to
> within 1px at the fifth, and text-heavy parity falls from 11.0% to 4.98% —
> what remains is genuine rasterizer difference, which does not accumulate.
>
> The cost is real and was measured rather than argued. Solid ink drops from
> 33.9% of ink pixels to 28.6%, midtones rise 46.0% to 56.7%: text is softer,
> landing between the old GPU output and the CPU's 22.7%/58.4%. Reviewed by eye
> at 9px and 15px as well as by number; at 15px the three are hard to tell
> apart, and at 9px the old output is visibly *wider* rather than visibly
> crisper. This is also what browsers, CoreText and Skia all do, and what gophics
> already did whenever LCD was active — so it is not a new class of appearance,
> just a wider application of one that already shipped.
> A grounded pass over the whole pipeline, generalizing
> `design/strip-rendering.md`.
>
> **The optimization target is GPU rendering on Metal and Vulkan.** The CPU
> rasterizer stays the correctness oracle, the no-GPU floor, and the
> measurement baseline.
>
> **The finding that organizes everything else:** tier 2b — stencil-then-cover —
> is the only tier without its own anti-aliasing. That gap is why the renderer
> runs 4× MSAA, and 4× MSAA is why `LoadOpLoad` is illegal, and *that* is why
> multi-pass frames corrupt on tile-based GPUs **and** why damage-rect
> scissoring is refused on every vector frame. Tier 2b is also where every
> stroke and every curved fill lands, and where 6 GPU objects are created and
> destroyed per path per changed frame. One root cause, four symptoms (§3).
>
> **Rev 3 adds §4–§5: how this work gets measured and grounded**, built on the
> frame-stats and device-counter spine that landed in `e18bee7`, plus the
> per-phase exit gates that turn each phase from an intention into a claim
> something can falsify. §4.2 names five gaps in that spine — the largest being
> that **F1, the plan's headline finding, is invisible to every instrument in
> the repo today** — and Phase A (§6) closes each one as a scoped work item
> with its call sites.

---

## 1. The pipeline as it actually is

### 1.1 Tiers

`GPURenderContext` (`gpu_render_context.go`) queues backend-agnostic
`drawCommand`s, tessellating at queue time rather than at flush
(`preTessellateFill`, ADR-051 — the fix that took animated paths from 3 to 60
FPS). `GPURenderSession` (`render_session.go`) builds per-frame resources and
records draws in a **fixed tier order per scissor group**
(`recordGroupDraws`, `render_session.go:2823`):

| Tier | Pipeline | Reached by | Own AA? |
| --- | --- | --- | --- |
| clip | `depth_clip.wgsl` | non-rect clips, before content | n/a |
| 1 | SDF (`sdf_render.wgsl`) | detected circles, ellipses, rects, rrects | **yes** — analytic smoothstep, 1px ramp |
| 2a | convex (`convex.wgsl`) | closed, single-contour, curve-free, convex, NonZero | **yes** — per-vertex coverage ramp |
| 2b | **stencil-then-cover** (`stencil_fill.wgsl` + `cover.wgsl`) | **everything else** | **no** — MSAA only |
| 3 / 3b | textured quad | CPU images, existing GPU textures | n/a |
| 4 | MSDF text | `DrawText` | yes — MSDF |
| 6 | glyph mask | `DrawGlyphMaskText`, LRU atlas | yes — CPU alpha atlas |

Tier order is fixed regardless of submission order, so z-ordering across tiers
depends on group boundaries. `drawCommand.sortKey` is reserved and unused.

### 1.2 What tier 2b actually catches

`extractConvexPolygon` (`gpu_types.go:37-76`) requires no curves, closed,
exactly one `MoveTo`, ≥3 points, and convex. And `preTessellateFill`
(`gpu_render_context.go:1474`) **skips the convex check entirely when the fill
rule is EvenOdd**.

`preTessellateStroke` sets `cmd.paint.FillRule = gg.FillRuleEvenOdd`
(`gpu_render_context.go:1532`) — and calls `preTessellateFill` three lines
later. So:

- **every stroke** — borders, dividers, hairlines, chart lines, focus rings,
  underlines — goes to tier 2b unconditionally, without the convex test even
  being attempted;
- **every curved fill** not caught by SDF shape detection goes there too.

Tier 2b is not the exotic case. **Measured (A2, Metal): 23% of drawn items on a
realistic themed screen, 75% on stroke-heavy content and 92% on curved fills —
against 0% for text.** The original claim here was "it is most of a UI", which
the corpus does not support for a real screen and which is not what makes it
matter: 23% of items is 12 paths, and 12 paths is 111 GPU objects created and
destroyed per frame (§2.1).

Note also what does *not* reach this tier. `theme.Divider` — the commonest rule
in this UI — is a filled `Decorated` rect, not a stroke, so it lands in SDF. The
"every divider is a stroke" intuition behind this section is wrong for gophics'
own widgets.

### 1.3 The compute path and the CPU path

`VelloAccelerator` is correct and reachable only via `PipelineModeCompute`;
`SelectPipeline` (`pipeline_mode.go`) returns `PipelineModeRenderPass`
unconditionally behind a measured table where render pass wins every cell by
1.4×–7× on Metal. That stands, and this plan does not revisit it.

`raster.AnalyticFiller` rasterizes every glyph
(`text/glyph_mask_rasterizer.go`), every fallback shape, and every frame on a
host with no WebGPU. It is the reference `app/gpu_equiv_test.go` diffs against.

### 1.4 MSAA

`resolveSampleCount` (`gpu_shared.go:416`) probes for a 4-sample texture and
uses **4× MSAA whenever the device supports it**, falling back to 1×
(`strategyNoMSAA`, `gpu_shared.go:34-38`) only when MSAA texture creation fails.

Per §1.1, every tier except 2b already anti-aliases itself. **4× MSAA exists to
anti-alias tier 2b**, and every other tier pays for it: 4× the color
attachment, 4× the depth/stencil attachment, and a resolve every pass. On a
tiler — the Vulkan target — that is the dominant bandwidth term.

---

## 2. Where the GPU frame actually goes

Ranked by expected value. Everything here is read out of code, not measured;
§4 is how that changes.

### 2.1 Tier 2b recreates 6 GPU objects per path, per changed frame — **F1**

`buildStencilResourcesBatch` (`render_session.go:1636`) keeps a slice-indexed
pool and then, for every path, every time it runs:

```go
// Destroy old pooled entry and create fresh buffers.
// Stencil paths vary wildly per frame (different vertex counts, colors),
// so recreating is simpler than capacity tracking for 6 sub-buffers.
if s.stencilBufPool[i] != nil { s.stencilBufPool[i].destroy(); … }
bufs, err := s.stencilRenderer.createRenderBuffers(…)
```

`createRenderBuffers` (`stencil_renderer.go:273`) allocates 4 buffers (fan
vertices, cover vertices, stencil uniform, cover uniform) and 2 bind groups;
`destroy()` (`stencil_renderer.go:192`) releases all six. Plus two
`make([]byte, …)` for the uniforms per path.

What that costs, precisely:

- **Metal**: `hal/metal/device.go:118` calls
  `[MTLDevice newBufferWithLength:options:]` per buffer, inside its own
  autorelease pool — a real driver allocation plus an ObjC bridge crossing.
- **Vulkan**: `hal/vulkan/device.go:310` calls `vkCreateBuffer` then
  suballocates from a **buddy allocator** over 64 MB blocks
  (`hal/vulkan/memory/`), so `vkAllocateMemory` is not per buffer. Cheaper than
  Metal, not free — a create/bind/free triple per buffer, and bind groups go
  through `descriptorAllocator.Allocate`/`Free` (`hal/vulkan/device.go:1036`)
  per set, per path, per frame.
- **Both**: every `Buffer` and `BindGroup` registers a `runtime.AddCleanup`
  (`wgpu/buffer.go:277`, `wgpu/bind_native.go:259`) and stops it on `Release`,
  so the churn is Go GC bookkeeping as well as driver work.

**There is a measured precedent for exactly this bug, in this repo.**
`offscreenPool` (`offscreen_pool.go`, landed `677927f`) was written because the
backdrop-blur passes created and destroyed six textures every frame, and its
header records the number: *"a single blur measured p95 53ms against 7.5ms for
a frame without one, on an A15."* Same class of defect — per-frame create and
destroy of GPU resources — on textures instead of buffers, 7× frame time on a
phone. F1 is that bug in tier 2b, and `offscreenPool` is the shape of its fix.

The comment is honest about the reason — capacity tracking for six sub-buffers
was more work than recreating. Its neighbours are already right:
`buildConvexResources` (`render_session.go:1556`) is grow-only with a cached
bind group; `buildTextResources` pools per-batch uniforms.

**Note the ordering, which compounds it:** resources are built at
`render_session.go:813`, long before `BeginRenderPass` (3031/3276) and
`applyGroupScissorWithDamage` (3187). Paths that damage scissoring will discard
still pay full resource construction.

### 2.2 Two pipeline switches and two draws per stencil path — **F2**

`StencilRenderer.RecordPath` (`stencil_renderer.go:479`) records per path:
`SetPipeline` → `SetBindGroup` → `SetVertexBuffer` → `Draw`, then `SetPipeline`
→ `SetBindGroup` ×2 → `SetVertexBuffer` → `SetStencilReference` → `Draw`.
`recordGroupDraws` calls it in a bare loop (`render_session.go:2848`), so
pipelines alternate stencil/cover/stencil/cover across the group.

The alternation is not removable by sorting — stencil state must be consumed by
its cover pass before the next path overwrites it. What *is* removable is the
per-path bind-group rebinding, which collapses into F1's fix.

### 2.3 MSAA forces multi-pass corruption and blocks damage — **F3**

Both `encodeSubmitSurface` (`render_session.go:2722`) and
`encodeSubmitSurfaceGrouped` (`render_session.go:3249`) set
`colorLoadOp = LoadOpLoad` when `frameRendered` is true.
`render_session.go:960-963` already documents that this cannot work:
multisampled content is discarded after resolve.

Two consequences, both live:

1. **Corruption.** Confirmed on a Pixel 10 Pro (PowerVR, Vulkan): a two-flush
   frame drops the first flush's content entirely.
   `design/gpu-single-pass-surface.md` scopes a fix and **is not
   implemented** — there is no accumulator in `render_session.go`. Eight
   `flushGPUAccelerator()` sites (`context.go:261,596,1798,1848`;
   `text.go:130,608,630,655`) can each split a frame into another pass.
2. **Damage refused.** `render_session.go:966` logs
   *"damageRects ignored: MSAA render path requires full LoadOpClear"* — so
   damage-aware rendering works only on blit-only frames. **Any frame
   containing a vector shape re-renders in full**, no matter how small the
   change.

Both are downstream of MSAA, which is downstream of tier 2b having no AA.

### 2.4 The frame loop: skipped when static, whole-scene when not — **F4**

`shellHandler.present` (`app/present.go:90-103`) skips the GPU replay entirely
when the scene is unchanged and the target is the same — a static UI costs
nothing.

But when anything changes, `ReplayScene` replays the **whole** display list
(`app/app.go:642`; the comment at `present.go:99` says so: "The GPU rasterizes
the whole frame each time; replay in full"). Every path is re-queued, re-cloned
(`path.Clone()` at `gpu_render_context.go:1000`, again after stroke expansion at
`:1531`), re-tessellated, and its GPU resources rebuilt.

So F1's cost is proportional to the whole scene, paid on every animating,
scrolling, hovering or caret-blinking frame — precisely where jank is felt.

There is no cross-frame retention for vector content: `core.prev.Replay` hands
the GPU layer a display list with no stable per-path identity, and
`scene.LayerCache` caches CPU pixmaps by hash rather than GPU resources.
Retention is the right long-term answer and is a display-list change, not a
renderer change — §7.4.

### 2.5 GPU timing: what exists, and the Metal honesty bug — **F5**

The renderer uses no timestamp queries; timing is CPU wall-clock, which on a
GPU frame measures submission, not execution. §4 works around that; here is why
it has to.

- **Vulkan HAL has timestamps**: `hal/vulkan/query.go:40` maps
  `QueryTypeTimestamp`; `timestampPeriod` comes from `VkPhysicalDeviceLimits`
  (`hal/vulkan/device.go:270`).
- **Metal HAL advertises them and cannot do them**: `hal/metal/api.go:75`
  inserts `FeatureTimestampQuery` for any Metal3 device, but `CreateQuerySet`
  returns `hal.ErrTimestampsNotSupported` (`hal/metal/device.go:1041`) and
  `ResolveQuerySet` is an empty stub (`hal/metal/encoder.go:342`).
- **The public `wgpu` package has no QuerySet API at all** — no
  `Device.CreateQuerySet`, no `TimestampWrites` on `RenderPassDescriptor`, no
  `ResolveQuerySet` on `CommandEncoder`. Only `core` has a `QuerySetID`.

So wiring per-tier GPU timing means new API through `wgpu` and `core` **and** a
Metal HAL implementation. It is a project, not a prerequisite.

The Metal feature bit is a standing honesty bug of the same class as PLAN.md
item 1 — a capability that answers without meaning anything — on the primary
desktop target. (Adjacent, cosmetic: the doc comment for `CreateCommandEncoder`
at `hal/metal/device.go:1034` runs straight into `CreateQuerySet`'s.)

### 2.6 Dead pipelines — **F6**

- `HybridPipeline` (`sparse_strips_gpu.go:412`) plus `gpu_flatten.go`,
  `gpu_coarse.go`, `gpu_fine.go`, `gpu_fine_dispatch.go` and
  `shaders/{flatten,coarse,fine}.wgsl` — 3,386 lines of Go and 1,214 of WGSL,
  reachable only from each other and from tests. `gpu/doc.go:25-33` still
  documents it as *the* scene-rendering path.
- `shaders/strip.wgsl`, embedded at `shaders.go:23`, whose only accessor
  `GetStripShaderSource` (`shaders.go:112`) has no non-test caller.

Do **not** confuse `shaders/coarse.wgsl` and `shaders/fine.wgsl` with
`tilecompute/shaders/coarse.wgsl` and `tilecompute/shaders/fine.wgsl` — the
latter are the live compute path.

Plus the CPU-side tile filler and its dead strip generator
(`design/strip-rendering.md`).

---

## 3. The root cause

```
tier 2b (stencil-then-cover) has no analytic coverage
        ↓
4× MSAA is required to anti-alias it
        ↓
every other tier pays 4× attachment bandwidth + a resolve   [cost]
        ↓
MSAA attachments cannot LoadOpLoad
        ↓
multi-pass frames corrupt on tile-based GPUs                [F3a, confirmed on device]
damage-rect scissoring refused on every vector frame        [F3b, logged and ignored]
```

…while the same tier is independently the allocation hotspot (F1) and the
pipeline-switch hotspot (F2), and catches every stroke and curved fill (§1.2).

The tactical fixes — hoist the allocations, widen tier 2a — are worth doing on
their own and are low-risk. The strategic move is to give tier 2b
coverage-based AA so the renderer can drop to 1× MSAA, which removes the
bandwidth tax on every tier, dissolves F3a without the accumulator design, and
unblocks damage on vector frames, all at once. That is also what a sparse-strips
or compute-coverage rasterizer produces (§7.3).

---

## 4. Measurement: the instrument

Nothing in §2 is measured. This section is what turns each finding into a
number, and each phase into a claim something can falsify. It is deliberately
built on what already exists rather than on the timestamp-query project §2.5
rules out.

### 4.1 What already exists

**Correctness — in CI, on Metal:**

| Instrument | What it holds |
| --- | --- |
| `internal/renderref` | one reference scene exercising every paint primitive, reaching all four quadrants, inside clip/opacity/transform groups |
| `app/gpu_equiv_test.go` `TestGPUMatchesCPU` | GPU vs CPU per pixel; `chanTol = 32`, reports differing-pixel fraction and max channel diff |
| `app/rendermatrix_test.go` `TestRenderScaleConsistency` | same scene at 1×/2×/3× must be structurally identical once normalized |
| `internal/gfx/gg/testdata/golden/` | per-primitive golden corpus, incl. upstream Vello sparse-strips references |
| CI `framework GPU tests` job | `go test -tags gophics_gpu -v ./app/ ./paint/`, self-skips without an adapter, verbose so a skip is visible |
| `scripts/gates.sh` | cheap repo gates; run by CI and by `.githooks/pre-push` |

**Performance — partly built, `e18bee7`:**

| Instrument | What it holds |
| --- | --- |
| `core.FrameStats()` (`app/app.go:467`) | p50 / p95 / p99 / worst over a 60-frame ring, plus the worst frame's ops, blurs, and GPU objects made, against the **median** op count |
| `wgpu.DeviceStats()` (`wgpu/devicestats.go`) | process-wide textures and pipelines created; differenced across a frame at `app/app.go:1057`/`:1094` |
| `GOPHICS_PACING=1` | logs that summary each time the ring wraps — the on-device readout |
| `BenchmarkRasterCPU` / `BenchmarkRasterGPU` (`app/gpu_equiv_test.go:363`) | headless raster timing; the GPU one includes a full readback and says so |

The design of `FrameStats` is already the right one for this work, and its
comment says why: percentiles rather than a mean, because *"stutter is a handful
of frames far above the rest, and averaging them into the sixty good ones around
them is exactly how they stop being visible in the number while staying visible
on the screen."* Everything below extends that spine rather than replacing it.

### 4.2 The five gaps

Each is closed by a numbered item in Phase A (§6); this section states the gap,
Phase A states the work.

1. **`DeviceStats` counts textures and pipelines, not buffers or bind
   groups** — precisely what F1 churns. Until they are counted, **F1 is
   invisible to every instrument in the repo**, including the on-device pacing
   readout. → **A1**
2. **Tier populations are not counted.** How many paths land in 2b vs 2a vs SDF
   vs text is the claim §1.2 rests on, and the loop that would report it already
   computes the sum and discards the breakdown
   (`render_session.go:703-707`). → **A2**
3. **Render passes, draws, and pipeline switches are not counted.** F2's
   alternation, F3's pass count, and F3b's discarded-damage warning are all
   unobservable. → **A3**
4. **`strategyNoMSAA` is unreachable on hardware that supports MSAA.**
   `detectStrategy` (`gpu_shared.go:398`) derives from a device probe with no
   override, so the highest-information measurement in §4.5 cannot be run
   today. → **A4**
5. **Nothing is compared to anything.** Numbers print and vanish; there is no
   recorded baseline, so no run can fail for being slower. → **A5**

### 4.3 The corpus

Four scenes, chosen because §1.1's tier table predicts they diverge — if they do
not, the tier model is wrong and that is the most valuable thing the corpus
could tell us:

| Scene | Exercises | Predicted dominant tier |
| --- | --- | --- |
| stroke-heavy | dividers, borders, chart lines, focus rings | 2b (§1.2) |
| curve-heavy | filled curved paths not caught by SDF | 2b |
| text-heavy | long runs, mixed sizes, cold and warm atlas | 6 |
| mixed / real | `internal/renderref`, plus a gallery screen | all |

`internal/renderref` is already the shared scene for both correctness harnesses
and should be the mixed case, so correctness and performance are measured on the
same content. `examples/gpucheck` covers the GPU-only path.

Each scene runs in three modes: **static** (no change — should cost ~0 per
§2.4), **animating** (one small element changes each frame — the F4 case), and
**cold** (first frame after a device reset — the pipeline-compile case
`DeviceStats` was built for).

### 4.4 The baseline artifact

A checked-in `design/baselines/` file per backend, holding for each scene and
mode: frame p50/p95/p99/worst, tier populations, objects created by kind
(texture, pipeline, buffer, bind group), render passes, draws. Recorded by a
`go test -tags gophics_gpu -run TestRenderBaseline` that writes the file when
`-update` is passed and compares against it otherwise.

Two properties matter more than precision:

- **It records the machine.** A baseline is meaningless without the adapter
  name, backend, and OS version in the file. Metal on an M-series and Vulkan on
  a Pixel are different files, never merged.
- **Counts gate; times report.** Object counts, tier populations, pass counts
  and draw counts are deterministic for a fixed scene, so they can fail a run
  hard. Frame times vary — `pipeline_mode.go` records 2–3× run-to-run variance
  on PowerVR — so they are reported and trended, never a hard gate. A perf gate
  that flakes gets disabled, and a disabled gate is worse than none.

### 4.5 Timing without timestamp queries

Given §2.5, per-tier GPU time comes from outside:

1. **Whole-frame GPU time** via submit-then-`Queue.Poll()`
   (`wgpu/queue_native.go:202`) to a completed submission index. Coarse but
   honest, and it catches regressions that CPU wall-clock hides.
2. **External captures for per-tier timing** — Xcode's Metal frame capture and
   GPU counters on macOS, Android GPU Inspector or perfetto on the Pixel. Zero
   code, real per-pass numbers, and the answer timestamp queries would otherwise
   cost a project to get. Capture once per phase, attach to the phase's note,
   do not automate.
3. **The MSAA ablation** — `strategyNoMSAA` already exists
   (`gpu_shared.go:34`). Forcing it and re-running the corpus prices §3's
   bandwidth term with no new code, on both backends. This is the single
   highest-information measurement available today.

### 4.6 Where each finding gets its number

| Finding | Instrument | Confirms or kills it |
| --- | --- | --- |
| F1 — 6 objects per path | §4.2(1) buffer+bind-group counters, differenced per frame | count scales with tier-2b population, or it does not |
| F2 — pipeline churn | §4.2(3) switch count; external capture | switches ≈ 2× 2b paths |
| F3a — multi-pass corruption | pass count + the device repro in `gpu-single-pass-surface.md` | passes > 1 on any real frame |
| F3b — damage refused | the existing `slogger().Warn` at `render_session.go:966`, counted | how many real frames trip it |
| F4 — whole-scene replay | static vs animating mode in §4.3 | static ≈ 0; animating scales with scene, not with change |
| §1.2 — 2b catches most of a UI | §4.2(2) tier populations | the central premise of this plan |
| §3 — MSAA is the tax | §4.5(3) ablation | delta on Vulkan vs on Metal |

If §4.2(2) shows tier 2b is a small minority of real draws, §3 is wrong and
Phases C and D should be re-scoped before a line is written. That is the point
of measuring first.

---

## 5. How progress is reported

Three artifacts, each answering a different question.

**The baseline file (§4.4) answers "did this change help?"** Every phase updates
it in the same commit as its change, so the diff *is* the result. A phase whose
baseline diff shows no movement on the number it targeted did not land, whatever
the code says.

**A short ledger at the top of this doc answers "where are we?"** One line per
phase: status, the number it moved, the backend it was measured on, the date. It
replaces the STATUS block growing unreadable, and it is the thing to read before
picking up work. Follow the house pattern in `design/gpu-opacity-layers.md`,
whose STATUS block records what was verified, on what hardware, and what remains.

**The on-device pacing readout answers "is it actually better on a phone?"**
`GOPHICS_PACING=1` already prints p50/p95/p99/worst with the worst frame's scene
size and objects made. Once §4.2(1) lands it also prints buffers and bind
groups, and a phase's mobile claim is a before/after pair of those lines from a
real device — not a desktop benchmark.

**What is deliberately not tracked:** micro-benchmarks of individual functions.
`profiling_test.go` has them and they are useful when profiling, but they are
not evidence a frame got faster, and treating them as such is how a plan
optimizes something a frame never waits on.

---

## 6. Phases

Each phase states what it does and **what has to be true for it to be done**.
The gates are the §4 instruments; a phase without a moved number is not
finished.

### Phase A — Build the instrument

Six items. Together they close §4.2 and stand up §4.3–§4.4. This is the smallest
phase in the plan by code volume and the one everything else is evaluated
against.

#### A1 — Count buffers and bind groups

The gap that makes F1 invisible. Strictly parallel to what `e18bee7` already
did for textures and pipelines.

- `wgpu/devicestats.go`: add `buffersCreated` and `bindGroupsCreated` atomics
  beside the existing two.
- Increment at the four `Device` choke points that mirror the existing ones:
  `device_native.go:62` (`CreateBuffer`), `device_native.go:361`
  (`CreateBindGroup`), and `device_browser.go:38` / `:181`.
- **Change `DeviceStats()` to return a struct**, not a widening tuple. It
  returns `(textures, pipelines uint64)` today with two callers
  (`app/app.go:1057`, `:1094`); a `DeviceCounts{Textures, Pipelines, Buffers,
  BindGroups}` makes this the last signature change, and the next counter free.
- `app`: `frameMade` is a single `int32` ring and `FrameSummary.WorstMade` a
  single total (`app/app.go:511-529`). Widen the ring to hold the four counts so
  the worst frame reports *what kind* of object it made — "made 312 objects" and
  "made 312 buffers and bind groups in tier 2b" are different diagnoses.
- Extend the `GOPHICS_PACING` line (`app/app.go:1101`) with the breakdown.

**Caveat to write into the doc comment:** these are process-wide atomics
differenced across a frame, so an app with a second context or window attributes
that work to this frame. Correct for the single-window measurement corpus,
wrong as a general accounting, and the existing comment should say so.

#### A2 — Count tier populations

The claim in §1.2 — that tier 2b catches most of a UI — is the premise the plan
rests on, and nothing reports it. **The loop already exists**:
`RenderFrameGrouped` sums exactly these seven populations at
`render_session.go:703-707` to decide whether the frame is empty, then throws
the breakdown away.

- Keep the sum; also record per-tier counts (SDF, convex, stencil, image, GPU
  texture, MSDF text, glyph mask).
- Reset them in `BeginFrame()` (`render_session.go:361`), which already exists
  as the frame boundary.
- **Surface them through `gg`, not `wgpu`.** These are gg concepts, and `app`
  cannot import `internal/gfx/gg/internal/gpu` — it is internal to `gg`. The
  `gpu` package already imports `gg`, so counters living in `gg` (the shape
  `RegisterCoverageFiller` already uses) are written by `gpu` and read by `app`
  with the dependency arrow unchanged.

#### A3 — Count passes, draws, and pipeline switches

Same home as A2.

- **Render passes** at the seven `BeginRenderPass` call sites
  (`render_session.go:2509, 2749, 3031, 3152, 3276, 3347, 3403`). F3's whole
  argument is a pass count and nothing counts passes.
- **Draws and pipeline switches** in `recordGroupDraws`
  (`render_session.go:2823`) and `StencilRenderer.RecordPath`
  (`stencil_renderer.go:479`). F2 predicts switches ≈ 2× the tier-2b
  population; that is a falsifiable statement once both are counted.
- **Damage-refused frames**: the `slogger().Warn` at `render_session.go:966`
  already fires when a damage rect is discarded because the frame is not
  blit-only. Count it. "How many real frames trip F3b" is a number nobody has.

#### A4 — The MSAA ablation hook

§4.5(3) is the highest-information measurement available and there is currently
no way to run it: `detectStrategy` (`gpu_shared.go:398`) derives the strategy
from `s.sampleCount`, which comes from `resolveSampleCount` probing the device
(`gpu_shared.go:416`), with no override.

One early return in `resolveSampleCount` behind an env var — matching the
existing `GOGPU_TEXT_NO_LCD` and `GOGPU_TEXT_DEBUG` precedent in this package —
makes `strategyNoMSAA` reachable on hardware that supports MSAA, which is what
the ablation needs.

#### A5 — The corpus and the baseline test

- **Scenes** (§4.3): extend `internal/renderref` with `StrokeHeavy()`,
  `CurveHeavy()` and `TextHeavy()` beside the existing `Scene()`. Putting them
  in `renderref` rather than a new package means correctness and performance are
  measured on the same content, and `TestGPUMatchesCPU` /
  `TestRenderScaleConsistency` pick up the new scenes for free.
- **Modes**: static, animating, cold (§4.3).
- **The test**: `app/renderbaseline_test.go` behind `gophics_gpu`, self-skipping
  without an adapter like the existing GPU tests. `-update` writes
  `design/baselines/<backend>-<adapter>.md`; without it, compares and fails on
  count regressions only (§4.4: counts gate, times report).
- **CI**: add it to the existing `framework GPU tests` job
  (`.github/workflows/ci.yml:169`). It self-skips, so a runner without an
  adapter is no worse off than today.

#### A6 — Fix the Metal timestamp feature bit

Independent of the rest, and it should not wait behind this phase.
`hal/metal/api.go:75` advertises `FeatureTimestampQuery` for any Metal3 device
while `CreateQuerySet` returns `ErrTimestampsNotSupported`
(`hal/metal/device.go:1041`) and `ResolveQuerySet` is an empty stub
(`hal/metal/encoder.go:342`). Either implement it over Metal counter sample
buffers or stop inserting the feature. The honest one-line version is fine and
strictly better than the current state — a caller can then detect the absence
instead of being told a lie. Fix the doc-comment collision at
`hal/metal/device.go:1034` while there.

**Done when:** `design/baselines/{metal-*,vulkan-*}.md` exist with all four
scenes in all three modes; the MSAA ablation is recorded for both backends; the
baseline test runs in CI; and every row of §4.6 reads as a number rather than an
inference — in particular F1, which A1 makes observable for the first time.

### Phase B — Cleanup and honesty

No behavioural risk; runs in parallel with A.

1. **Delete the dead GPU pipelines** (F6): the `HybridPipeline` cluster, its
   three shaders, its tests, and the `HybridPipeline` section of `gpu/doc.go`.
   Remove the `strip.wgsl` embed and `GetStripShaderSource` — keep the `.wgsl`
   file and the naga compile fixture at
   `internal/gfx/naga/spirv/internal/codegen/shader_test.go:2462`, which is real
   WGSL front-end coverage and which §7.3 may need.
2. **Execute `design/strip-rendering.md` Phase 0**: rename the CPU tile filler
   to what it is, delete its unreachable strip generator and dead SIMD, write
   the missing multi-contour regression test, fix the four wrong comments in
   that doc's §2.
3. **Fix the Metal doc-comment collision** (`hal/metal/device.go:1034`).
4. **Retire `drawCommand.sortKey`** or use it.

**Done when:** ~4,100 lines of non-test Go, 2,400 of test and 1,200 of WGSL are
gone; `gates.sh` and the full test suite are green; and the baseline file is
byte-identical before and after — a cleanup that moves a number deleted
something live.

### Phase C — Tier 2b, tactical

Contained and low-risk. Each step gated on `TestGPUMatchesCPU` staying green and
on the baseline moving the number it targeted.

1. **Hoist tier 2b's resources out of the frame loop (F1).** One grow-only
   vertex arena for fan and cover geometry with per-path offsets; one uniform
   buffer with dynamic offsets; bind groups created once per layout. The shape
   is `offscreenPool`'s (§2.1) and the target is `buildConvexResources`'s: zero
   GPU object creation per changed frame in steady state.
2. **Let strokes reach tier 2a.** `preTessellateFill` skips the convex test for
   EvenOdd, three lines after `preTessellateStroke` sets EvenOdd. This looks
   vestigial rather than intentional: `AnalyzeConvexity` (`convexity.go:60`)
   carries a direction-flip guard added **specifically to reject
   self-intersecting stroke outlines**, so the path was built and the EvenOdd
   gate made it unreachable. The expander emits `VerbClose`
   (`stroke/expander.go:832`), so outlines are closed. And on a verified convex
   simple polygon EvenOdd and NonZero are equivalent — a ray crosses the
   boundary once, winding is ±1 — so routing on *verified convexity* is correct
   regardless of the declared rule.

   Scope honestly: closed-path strokes expand to two contours and are rejected
   by `moveCount != 1`; round joins and caps produce curves and are rejected by
   `hasCurves`. What qualifies is open strokes with butt or miter joins —
   dividers, underlines, borders drawn as lines, chart segments.
3. **Collapse the per-path bind churn (F2)** — falls out of C1. Measure whether
   more is worth it only after C1.
4. **Drop the per-draw `path.Clone()`** if the baseline puts it on the profile.

**Done when:** buffers and bind groups created per changed frame are **zero in
steady state** on both backends; tier-2a population rises and 2b falls by the
stroke count on the stroke-heavy scene; frame p95 on the animating mode improves
on the Pixel; `TestGPUMatchesCPU` and the golden corpus unchanged.

### Phase D — Tier 2b, strategic: coverage instead of MSAA

The §3 root cause. Give tier 2b its own analytic coverage so `sampleCount` can
drop to 1, then take the three payoffs: attachment bandwidth on every tier,
`LoadOpLoad` becoming legal (F3a dissolves without the accumulator), and
damage-rect scissoring on vector frames (F3b).

**Gated on Phase A's MSAA ablation.** If 4× MSAA is cheap on both backends this
phase is not worth its risk and the fallback below is the answer. If it is
expensive on the Pixel — the expectation — this is the largest item in the plan.

Candidate mechanisms, chosen on evidence rather than now:

- **CPU coverage strips uploaded as a mask**, composited by a lightweight GPU
  tier — `design/strip-rendering.md` Phases 3–5, and what PLAN.md §5 meant by
  "CPU geometry and coverage in SIMD-friendly strips, plus lightweight GPU
  compositing." `shaders/strip.wgsl` is the starting point.
- **GPU compute coverage** for this one tier. `SelectPipeline`'s table says the
  whole-scene compute pipeline loses badly; one tier is a different measurement
  and nobody has taken it.
- **Analytic coverage in the cover pass.** Cheapest to try, most likely to fail:
  stencil-then-cover is fundamentally binary in/out.

**Done when:** `sampleCount` is 1 on the corpus with no golden regression beyond
the agreed AA tolerance — reviewed by eye, not only by `chanTol`, because this
phase changes edge quality by construction; the MSAA bandwidth delta from Phase
A is realized on the Pixel; and the F3b warning at `render_session.go:966` stops
firing on vector frames.

**Fallback if D is not taken:** implement `design/gpu-single-pass-surface.md` as
written — single-sample accumulator plus present blit, and capturing CPU
fallbacks as ordered texture draws rather than mid-frame flushes. That fixes
F3a's confirmed corruption without touching MSAA, and recovers neither the
bandwidth nor damage on vector frames.

### Phase E — Damage-rect present

`paint.PresentGPU` (`paint/paint.go:597`) passes an empty `image.Rectangle{}`,
dropping damage the app already computed (`app.go:633`). `context.go:1459`
quantifies the miss: a 48×48 spinner should update 9 KB, not 8 MB at 1080p.

The trap is the job: the two-frame union that makes partial damage safe against
a recycled swapchain image lives in `ggcanvas.Canvas` (`canvas.go:68`,
`canvas.go:361-397`, the Wayland buffer-age pattern), and `PresentGPU` bypasses
it. Single-frame damage into a double-buffered surface leaves pixels from two
frames ago.

Sequenced after D because today this only helps blit-only frames (§2.3).

**Done when:** the animating mode's uploaded bytes drop to the damage rect's
area on both backends, and a two-frame-stale-pixel test — render, change a small
region, present twice, read back — passes on the Pixel.

### Phase F — CPU-only floor

`design/strip-rendering.md` Phases 1–2 (the benchmark and the routing decision)
are cheap and should run early; Phase 1's likely outcome, "delete the routing,"
is Phase B work. Its Phases 3–5 are now **inputs to Phase D**, not a separate
CPU-optimization track (§7.3).

---

## 7. Judgment calls, including two reversals

### 7.1 Why the instrument comes before the fixes

Every number in §2 is read out of code. F1 could be largely absorbed by Metal's
own allocator and severe on Vulkan's descriptor pool, or the reverse — and
§4.2(1) says it is currently invisible to every instrument in the repo,
including the on-device readout. Phase A is small and settles it. Phase B has no
behavioural risk and should not wait for anything.

### 7.2 Why C before D

C is contained inside one renderer with an existing parity gate. D changes the
sample count and therefore the visual output of every stencil path — the
highest-risk change in the plan. Doing C first also makes D's payoff measurable:
with allocation noise gone, an MSAA change shows up cleanly.

### 7.3 Reversal: strips are not a CPU-only concern

An earlier revision said strips "make the no-GPU floor faster and are worth
nothing" to GPU work, and sequenced them last on that basis. That was wrong.

A rasterizer that produces *coverage spans* is the natural input to a GPU tier
that composites coverage — exactly what tier 2b lacks (§3), and the lack is what
forces 4× MSAA. Strips are therefore a candidate for the strategic fix on the
GPU target, not only an optimization for hosts without one. PLAN.md §5 already
framed it that way; the earlier revision lost the thread by scoping strips
against `AnalyticFiller` instead of against stencil-then-cover.

The sequencing barely changes — strips remain gated on measurement and behind
Phase C — but the reason does, and so does what the eventual benchmark must
answer: `design/strip-rendering.md` Phase 1 should measure coverage-generation
throughput, not only fill speed against `AnalyticFiller`.

### 7.4 Retention is the right long-term answer and is not scheduled here

§2.4: any changed frame re-queues, re-clones, re-tessellates and rebuilds GPU
resources for the entire scene. Caching that across frames is what Skia and
Impeller do, and it would subsume F1, F2 and much of E.

It is not proposed as a phase because the blocker is not in the renderer.
`core.prev.Replay` hands the GPU layer a flat display list with no stable
per-path identity, and `scene.LayerCache` caches CPU pixmaps by hash rather than
GPU resources. Plumbing identity or generation counters out of the display list
is a `scene`/`app` design change deserving its own doc, scoped after Phase A
says what it would be worth.

---

## Non-goals

- **Reviving the whole-scene compute pipeline.** `SelectPipeline`'s table
  stands. Using compute for *one tier* (Phase D) is a different question.
- **Replacing `AnalyticFiller`.** It remains the reference the GPU path is
  diffed against, whatever wins on speed.
- **Public API changes.** The rasterizer cluster is under `internal/`; the only
  reference from a public package is `paint/paint.go:917-922`, a workaround
  Phase B deletes.
- **Hard perf gates in CI.** §4.4: counts gate, times report. A flaky perf gate
  gets disabled, and a disabled gate is worse than none.

## Risk

Phase C touches the tier that draws most of a UI, and a wrong buffer offset
produces plausible-looking wrong output rather than a crash — hence the parity
gate on every step. Phase D changes anti-aliasing quality by construction and
needs golden review by eye, not just a numeric threshold. Phase E is on the
shared present path for every platform.

The failure mode for the plan as a whole is starting at Phase C or D because §3
reads convincing. It is convincing, and it is still a chain of inferences from
reading code. Phase A is small, and §4.6 says exactly which of those inferences
it would break.
