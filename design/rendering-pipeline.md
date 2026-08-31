# Rendering pipeline — improvement and cleanup plan

> **STATUS 2026-08-31 (rev 5).** Phase A landed, C1 landed, Phase D closed as
> not justified, Phase E done. Measured on Metal (M1 Ultra) and Vulkan
> (Mali-G615, Galaxy Tab A9+) unless noted.
>
> | Item | Status | What it moved |
> | --- | --- | --- |
> | A1 buffer/bind-group counters | done | made F1 observable at all |
> | A2 tier populations | done | corrected §1.2 — see below |
> | A3 passes / draws / switches | done | confirmed F2 |
> | A4 MSAA ablation hook | done | priced MSAA on both backends |
> | A6 Metal timestamp honesty | done | removed a feature bit that lied |
> | C1 hoist tier 2b allocations | done | 121 → 1 objects/frame; −45% frame time |
> | glyph bind-group reuse | done | 38 → 0 bind groups/frame |
> | C2 strokes → tier 2a | **reverted** | worked, but GPU/CPU parity 0.48% → 10.27% |
> | D coverage-AA instead of MSAA | **closed** | not justified; see below |
> | E damage-rect present | done | CPU present 8,480 → 230 KB/frame |
> | F7 text layout divergence | done | GPU drift 9px → 0 |
> | opacity single-draw fold | done | 21 → 1 passes; Mali 54 → 12 ms |
> | A5 baseline file | not started | — |
> | B remaining cleanup | partly done | 5,839 lines of dead pipeline removed |
>
> **What the measurements changed about the model.**
>
> - **F1 and F2 are real and exact.** Tier 2b created 4 buffers + 2 bind groups
>   per path per frame, in steady state with no pipeline creation beside it, so
>   none of it amortized. Pipeline switches land within two of 2× the stencil
>   population.
> - **§1.2 was overstated.** "Tier 2b catches most of a UI" holds for stroke-
>   and curve-dominated content (75%, 92% of items) and not for a real screen:
>   `renderref.UIScreen()` is 23% tier 2b. `theme.Divider` is a filled rect, not
>   a stroke, so the commonest rule in this UI lands in SDF.
> - **One opacity group costs one render pass.** Clips, gradients and sprites
>   cost none. That, not tier 2b, is what made the reference scene 21 passes.
> - **MSAA is free once the pass count is low.** It appeared to cost 2× on the
>   tiler; that was `passes × per-pass resolve`, and folding single-draw opacity
>   groups removed it. Phase D's premise — that MSAA taxes every tier to
>   anti-alias tier 2b — does not survive: the tax was proportional to passes,
>   and a forty-line fold fixed it with no quality cost.
>
> **Still open from §2/§3:** F3a's multi-pass corruption is real and wants
> `design/gpu-single-pass-surface.md`; F3b (damage scissoring refused on vector
> frames) is still refused, and worth re-measuring now that a UI frame is one
> pass. C2 is blocked on tier 2a's AA quality rather than on its routing.
>
> **Running the GPU suite on a phone**, which is cheaper than it looks — the
> zero-CGo build means no app, no JNI, no gomobile:
>
>     CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -tags gophics_gpu -o app.test.android ./app
>     adb push app.test.android /data/local/tmp/ && adb shell chmod +x /data/local/tmp/app.test.android
>     adb shell "cd /data/local/tmp && ./app.test.android -test.run TestMSAAAblation -test.v"
>
> Prefix `GOGPU_NO_MSAA=1` for the ablation. Headless Vulkan comes up against
> `vulkan.mali.so` directly.
>
> **Two instrument traps worth knowing**, both of which produced confident wrong
> answers here: frame time measured through `RenderToImage` charges a full
> surface readback every frame (575 KB against 20 KB uploaded), which hides
> anything damage-related; and `getImageData` on the web canvas returns
> all-black through browser automation while the page renders correctly, so
> pixel sampling reads as "nothing changed".
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

> **STATUS 2026-08-30 — blocked on an instrument, not on the work.** Sizing it
> was attempted and could not be done with what exists.
>
> | | Metal (M1 Ultra) | Vulkan (Mali-G615) |
> | --- | --- | --- |
> | full scene | 1.09 ms | 7.40 ms |
> | full scene, one element changing | 1.08 ms | 9.73 ms |
> | same scene clipped to 48×48 | 1.05 ms | **8.60 ms** |
>
> Clipping to 1% of the area is *slower* than not clipping on the tiler. That is
> not a result about scissoring: the headless harness renders through
> RenderToImage, which reads the whole surface back every frame, and that
> readback is proportional to the surface, cannot be removed by damage, and
> swamps what damage would save.
>
> The phase's own "done when" asks for **uploaded bytes** to drop to the damage
> rect's area, and nothing counts uploaded bytes. Frame time measured through a
> readback harness is the wrong proxy and would report success or failure at
> random — which is exactly the trap §7.1 says the instrument exists to avoid.
>
> **DONE 2026-08-31, and smaller than the plan thought.** Most of Phase E as
> written no longer describes the code: `paint.PresentGPU` does not exist (the
> paint package no longer imports shell), the two-frame damage union the plan
> called "the trap" already exists and is live in
> `ggcanvas.forwardDamageRects`, and the web CPU present already honoured
> damage. What was actually left was one path —
> `shell/desktop/present.go`, which created a whole new texture per frame and
> discarded the rect it was handed.
>
> Retaining the texture and uploading only the damaged rows:
>
> | | per frame |
> | --- | --- |
> | before | 8,480 KB |
> | after | **230 KB** |
>
> **37× less uploaded**, on a 640×640 window at 2× with one label changing.
>
> **Verified in Chrome at devicePixelRatio 2** (2230×2434 canvas, CPU renderer):
> two consecutive frames of an animating scene differ across the entire height,
> including physical rows 1400–2030 — well below the 1217 halfway line the
> pre-fix code never uploaded. No stale pixels.
>
> Worth recording how, because the obvious instrument does not work:
> `getImageData` on this canvas returns all-black through the extension even
> while the page renders correctly, so pixel sampling reads as "nothing changed"
> and would have reported the bug as present after it was fixed. Screenshots are
> the reliable readback here.
>
> **A HiDPI bug found on the way, and it was mine.** `PixelTarget.Put` receives
> the *physical* surface, and the damage rect was being passed in *logical*
> coordinates — so at 2× it named the top-left quadrant's rows. The web
> presenter shipped with this in Phase 4a: it uploaded half the height and left
> the rest holding the previous frame. `app.present` now scales the rect where
> the coordinate space changes, and the first "48% saving" this work appeared to
> show was entirely that bug uploading half a surface.
>
> **A second finding, still open:** a text op whose font has no metrics produces
> a zero-height damage rect, and `recordScene`'s "degenerate bounds → repaint
> everything" fallback then silently forces a full-surface upload. It cost a
> real measurement here — an app with no Font configured pays full damage on
> every frame that touches text and nothing says so.
>
> **The rest of the original note, kept because the reasoning still applies.**
> Building Phase E first would mean changing buffer-age and two-frame-union
> semantics with no way to show it helped, which is how a risky change gets
> merged on faith.
>
> **The counter now exists** (`wgpu.TransferStats`, counted at WriteBuffer,
> WriteTexture and MappedRange — the only ways data reaches or leaves a device),
> and it priced the problem on the first run. Identical on Metal and Mali:
>
> | | per frame |
> | --- | --- |
> | uploaded | **20.6 KB** — all buffer, no texture |
> | read back | **575.0 KB** — 320×460×4, the whole surface |
>
> Two things follow, and they change what Phase E is for.
>
> The readback is 28× the upload and is the full surface every frame regardless
> of what changed. That is why frame time could not see scissoring: the harness
> was paying a fixed 575 KB that damage cannot touch. It is a property of the
> headless harness rather than of an app, and it means **frame time from
> RenderToImage is not a usable signal for any damage work**.
>
> The uploads are 20.6 KB of vertices and uniforms and **zero texture bytes**.
> So `context.go:1460`'s "8 MB at 1080p" is not describing this path at all — it
> describes the CPU present path, which uploads the surface with WriteTexture.
> On the GPU render path the surface is rendered in place and never uploaded, so
> Phase E's value there is scissoring fragment work, not saving bandwidth. Those
> are different wins with different sizes, and the plan conflated them.


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
