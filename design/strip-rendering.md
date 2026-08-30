# Strip rendering — what's actually there, and whether to build it

> **STATUS 2026-08-28 — ANALYSIS ONLY, nothing implemented. SUPERSEDED FOR
> SEQUENCING by `design/rendering-pipeline.md`.** gophics does not do strip
> rendering. Three things in the tree wear the name; none renders a pixel in a
> shipping frame. The real rasterizer on every path that matters is
> `raster.AnalyticFiller` (Skia analytic AA, 256 levels, `AlphaRuns` spans).
> This doc grounds that claim in code, corrects four comments that assert
> otherwise, and scopes the work in phases.
>
> **What changed on 2026-08-28.** A wider grounding pass put the optimization
> target on GPU rendering for Metal and Vulkan, where the CPU rasterizer is the
> correctness oracle, the no-GPU floor, and the measurement baseline — but not
> something to make fast. Strips are the only item on any list that speeds up
> the floor, and they are worth nothing to the other two jobs. So:
>
> - **Phase 0 (honesty pass) is promoted** — it is Phase B of the umbrella plan
>   and should happen now, in parallel with everything else.
> - **Phase 1 (the missing benchmark) stays cheap and worth running early**, and
>   its likely outcome ("delete the routing") is also Phase B work.
> - **Phases 3–5 (building the generator) are demoted to Phase F**, behind every
>   GPU hot-path item. The §4 debate below is kept because its reasoning is
>   still correct; its verdict has been overtaken by larger GPU findings —
>   see `design/rendering-pipeline.md` §2.
>
> Feeds PLAN.md §5, which names the sparse-strips upgrade as the next GPU
> vector item but does not scope it.

## 1. What is true today

### 1.1 The CPU filler is a tile rasterizer, not a strip rasterizer

`internal/gfx/gg/internal/gpu/sparse_strips.go` has the right pipeline shape —
flatten, coarse bin, backdrop, fine — and `fine.go:719-1044` defines a real
`SparseStrip` and `StripRenderer` over a shared `alphas []uint8`. That is 326
lines of a genuine strip generator.

It never runs. The production entry point is `SparseStripsFiller.FillCoverage`
(`sparse_strips_filler.go`), and it calls `RasterizePath` — not
`RasterizeToStrips` — then walks the tile grid and invokes
`callback(x, y, coverage)` **once per pixel**. `RasterizeToStrips`
(`sparse_strips.go:147`), `Strips()` (`sparse_strips.go:187`) and every method
on `StripRenderer` have no non-test caller.

What runs instead is `map[uint64]*Tile` with a 16-byte coverage array per 4×4
tile (`tile.go:118`, `tile.go:26`). Three costs follow, and they are structural
rather than tuning problems:

- `CalculateBackdrop` (`coarse.go:354`) allocates `make([]int32, cols*rows)` —
  the **whole** tile grid, densely, per path fill. At 1000×1000 that is 62,500
  int32 = 250 KB per fill.
- `FineRasterizer.Rasterize` (`fine.go:69`) allocates a `visited` map per call,
  then `emitBackdropOnlyTiles` materializes every interior tile into the grid.
  A fully covered 1000×1000 fill is ~62,500 map inserts.
- The filler then emits ~1,000,000 closure calls for that fill. The scanline
  filler emits ~1,000 span writes.

Every property that makes strips fast — contiguous alpha runs, dense interior
encoded as two integers rather than pixels, wide SIMD across a run — is absent
from the path that executes. `sparse_strips_simd.go` (347 lines) vectorizes
16-pixel tiles, which is the wrong unit, and is referenced only by itself and
its tests.

### 1.2 It is in the default build, and guarded in four places

`paint/accel_link.go` blank-imports `internal/gfx/gg/gpu` under `!nogpu` — the
default build — and that package's `init` registers `AdaptiveFiller`
(`gg/gpu/gpu.go:38`). So the tile filler is live in every default binary. What
keeps it off real geometry is a set of guards, each added independently, none
with a regression test:

| Site | Guard |
| --- | --- |
| `software.go:265` `shouldUseTileRasterizer` | rejects any path with >1 contour, plus `minTileArea=512`, `minSingleDimension=8`, `nElems > 2048/√area` clamped to [32, 256] (`software.go:174-193`) |
| `software.go:1277` `SoftwareRenderer.Stroke` | forces `RasterizerAnalytic` around the stroke-expanded fill |
| `gpu_render_context.go:1625` `dispatchDrawsToSoftware` | forces `RasterizerAnalytic` for `drawCmdStrokePath` |
| `paint/paint.go:922` `Painter.runFor` | forces `RasterizerAnalytic` for glyph-run scratch contexts |

Each cites the same defect: the tile filler mishandles multi-contour winding
and fills the gaps between contours solid. Four workarounds, zero tests. What
remains reachable after the guards is single-contour blobs of moderate size and
high vertex count — uncommon in UI work.

### 1.3 The GPU strip shader is never compiled, and neither is the pipeline below it

`shaders/strip.wgsl` (155 lines) is embedded at `shaders.go:23`, but
`GetStripShaderSource()` (`shaders.go:112`) has no non-test caller and no
pipeline is built from it.

Separately, `HybridPipeline` (`sparse_strips_gpu.go:412`) and the three GPU
stages it drives — `gpu_flatten.go`, `gpu_coarse.go`, `gpu_fine.go`,
`gpu_fine_dispatch.go`, with `shaders/flatten.wgsl`, `shaders/coarse.wgsl`,
`shaders/fine.wgsl` — are reachable only from each other and from
`sparse_strips_gpu_test.go`. That is 3,386 lines of Go and 1,214 lines of WGSL
that no frame touches.

Note the name collision: `tilecompute/shaders/coarse.wgsl` and
`tilecompute/shaders/fine.wgsl` are **different files**, used by
`vello_compute.go`, which is live under `PipelineModeCompute`. Do not delete
those.

### 1.4 The live renderers

GPU: the tiered render-pass pipeline — SDF shapes, convex tessellation,
stencil-then-cover, glyph-mask atlas, textured quads, depth clip, backdrop blur
(`render_session.go`). The Vello *compute* pipeline is correct but deliberately
not auto-selected; `SelectPipeline` (`pipeline_mode.go`) returns
`PipelineModeRenderPass` unconditionally and carries the measured table that
justifies it. That decision is about compute. It says nothing about strips.

CPU: `raster.AnalyticFiller` throughout, including every glyph
(`text/glyph_mask_rasterizer.go`). Zero-alloc edge building straight from
`float64` coords via `BuildFromPathF64`. It is good. This plan is not a rescue.

## 2. Corrections to the record

Four comments in the tree assert things the code does not do. They are why this
reads as finished work:

- `software.go:276` — "gg already does this for stroke-expanded paths (doStroke
  forces RasterizerAnalytic)". `Context.doStroke` (`context.go:1812`) does not:
  it passes the caller's mode through `tryGPUStrokeWithMode`
  (`context.go:2128`), which only ever returns `RasterizerSDF` or
  `RasterizerAuto`. The forcing is in `SoftwareRenderer.Stroke`
  (`software.go:1277`).
- `fine.go:721` — "different from the legacy Strip type in strips.go". There is
  no `strips.go`.
- `gpu/doc.go:25-33` documents the `HybridPipeline` as the scene-rendering path.
  It has never run.
- `gg/gpu/gpu.go:36` and `gg/raster/raster.go:7` describe "SparseStrips (4x4
  tiles, SIMD-optimized)". Neither strips nor SIMD are in the executed path.

## 3. Three decisions that shape the plan

**The existing code is not a head start.** The `map[uint64]*Tile` grid, the
dense per-fill backdrop allocation, and the per-pixel callback all have to go.
A strip generator replaces `CalculateBackdrop` outright. Roughly 4,100 lines of
non-test Go here get deleted or rewritten rather than extended. Treating it as a
starting point is what would make this expensive.

**Nothing here is public API.** `RasterizerMode`, `CoverageFiller`,
`SparseStripsFiller` and the whole cluster live under `internal/`. A scan of
every non-internal package finds exactly one reference — `paint/paint.go:917-922`,
and it is a workaround comment plus the call that applies it. Renaming and
deleting is free: no deprecation, no shim, no version bump.

**The routing has never been measured.** `SelectPipeline` earned its verdict
with a real cross-pipeline benchmark. The analytic-vs-tile call did not: the
thresholds at `software.go:174-193` are derived from reasoning about cache
behaviour, and no benchmark in the tree runs the two fillers over the same path.
Building strips before that benchmark exists means building without a baseline
to beat.

## 4. This work vs. damage-rect texture upload

`roadmap`/PLAN.md lists both. They are not the same kind of bet, and the honest
answer depends on which host you are optimizing.

### The case for damage-rect upload

It is a **per-frame, every-frame** cost on the path most desktop apps take, and
the repo already quantifies it: `context.go:1459` — "a 48×48 spinner updates
only 9KB instead of the full surface (8MB at 1080p)". Damage is tracked
(`app.go:633` `ReplayDamaged`), the raster is already damage-culled, and the
compositor already honours `DamageRects` (`context.go:1461`). The only thing
dropping it is `paint/paint.go:597`, which calls `FlushGPUWithViewDamage` with
an empty `image.Rectangle{}`.

That path is blit-only, so `LoadOpLoad` + scissor applies and no MSAA warning
fires (`context.go:1450-1457`). The trap PLAN.md names is real and is the whole
job: the two-frame union that makes partial damage safe against a recycled
swapchain image lives in `ggcanvas.Canvas` (`canvas.go:68`, `canvas.go:361-397`,
the Wayland buffer-age pattern), and `PresentGPU` goes to the context directly,
bypassing it. Single-frame damage into a double-buffered surface leaves pixels
from two frames ago — intermittent, invisible to tests, visible to users.

So: buffer-age accounting on a second path, in code that already implements the
pattern once. Bounded, non-novel, with a number attached.

### The case for strips

It is the **floor**, not the frame. PLAN.md §5 is explicit that the CPU
rasterizer "works anywhere, including headless CI and hosts with no WebGPU." On
those hosts `presentSurface` takes the `shell.PixelTarget` branch
(`app/present.go:127`) and there is no GPU upload at all — damage-rect upload
buys exactly nothing, and CPU rasterization is 100% of the frame. Strips are the
only item on either list that makes that host faster.

The directional bet is also sound and is the repo's own reading (PLAN.md:291-294):
Gio shipped a compute renderer derived from piet-gpu and retired it in January
2025; Linebender is betting on sparse strips over compute. Strips are the most
Go-portable design — far less compute-shader surface to trust — and they degrade
to the CPU backend because both share the generator.

### Where it loses

Two caching layers already absorb most of the workload strips would speed up.
Glyph masks are LRU-cached in `GlyphMaskAtlas` (`text/glyph_mask_atlas.go:383`),
and whole text runs are cached again by `Painter.runFor` (`paint/paint.go:887`)
keyed on font, string, size, colour and scale. Rasterization happens on cache
miss. The win is cold start, scale changes, and animated text size — not a
steady-state frame.

And a steady-state GPU frame is dominated by compositing and atlas upload, not
by CPU path filling. Strips will not transform it.

### Verdict (as of 2026-08-27; overtaken — see below)

**Damage-rect upload first**, on value per unit of risk: it is bounded, it is
not novel, its trap is already written down, and its win is quantified in-repo
and paid every frame. Strips have no number attached to them at all.

**But phases 0–2 below are not competing with it.** They are a cleanup and a
measurement, they are cheap, and phase 1 produces exactly the number that is
missing from this comparison.

**Update 2026-08-28.** Both sides of this debate were sized against each other
and not against the rest of the pipeline. A wider pass found larger, cheaper GPU
wins than either — chiefly that stencil-then-cover creates and destroys six GPU
objects per path per frame, and that every stroke and every curved fill lands in
that tier (`design/rendering-pipeline.md` §2.1–2.2). Damage-rect upload keeps
its place but moves behind the single-pass work that changes the pass structure
it would account against. The reasoning above stands; the ordering is now set by
the umbrella plan.

## 5. Phase 0 — Honesty pass

Independent of whether strips ever get built. The point is that a reader today
concludes the design decision is made and only tuning remains. It is not made.

1. **Write the failing test first.** A multi-contour fill (two glyphs, or one
   glyph with a counter) through the tile filler, asserting the gap stays empty.
   Four workaround sites cite this defect and none of them has a test. Whatever
   happens next, the guard should have a recorded reason.
2. **Rename to what runs.** `SparseStripsFiller` → `TileCoverageFiller`,
   `SparseStripsRasterizer` → `TileRasterizer`, `RasterizerSparseStrips` →
   `RasterizerTileCoverage`, files to match.
3. **Delete the unreachable strip machinery**: `SparseStrip`, `StripRenderer`
   and their methods (`fine.go:719-1044`, 326 lines); `RasterizeToStrips` and
   `Strips()` (`sparse_strips.go:146-190`); all of `sparse_strips_simd.go` (347
   lines).
4. **Delete the dead GPU cluster**: `sparse_strips_gpu.go`, `gpu_flatten.go`,
   `gpu_coarse.go`, `gpu_fine.go`, `gpu_fine_dispatch.go` (3,386 lines) and
   their tests (2,434 lines); `shaders/flatten.wgsl`, `shaders/coarse.wgsl`,
   `shaders/fine.wgsl` (1,214 lines). **Not** `tilecompute/shaders/*` — those
   are the live compute path. Drop the `HybridPipeline` section from
   `gpu/doc.go`.
5. **Remove the `strip.wgsl` embed and `GetStripShaderSource`.** Keep the
   `.wgsl` file and the naga compile fixture at
   `internal/gfx/naga/spirv/internal/codegen/shader_test.go:2462` — that test is
   real coverage of the WGSL front end, and its own failure message says a skip
   there is how these fixtures sat dead. The shader is phase 5's starting point.
6. **Fix the four wrong comments** listed in §2.
7. **Keep `testdata/golden/vello-sparse-strips/`.** Four reference images from
   upstream `vello_common::StripGenerator`, with a README recording expected
   diffs (0.90%–3.80%, curve-flattening and edge-AA precision). That is phase
   3's correctness oracle and it was not free to get.

Net: ~4,100 lines of non-test Go, ~2,400 of test, ~1,200 of WGSL. No behaviour
change — none of it executes.

## 6. Phase 1 — The benchmark that does not exist

Build the head-to-head before writing any strip code. It has to serve twice: as
the verdict on the current tile routing, and as the baseline any strip generator
must beat.

- Same path, same pixmap, same fill rule, through `AnalyticFiller` and through
  the tile filler.
- Sweep the axes the heuristic claims to care about: element count, bbox area,
  contour count, fill density, canvas size. Cover the region
  `shouldUseTileRasterizer` selects **and** the regions it excludes.
- Report ns/op and B/op. The 250 KB-per-fill backdrop allocation, the per-call
  `visited` map, and the per-pixel closure should all be visible in B/op; if
  they are not, that is worth knowing too.
- Add a **cold-cache text frame** case — `GlyphMaskAtlas` and `Painter.runFor`
  both cleared. Without it, phase 3 is justified by intuition.

Expected outcome, stated in advance so the result can contradict it: the tile
filler loses across the whole sweep, including inside its own selection window.

## 7. Phase 2 — Decision gate

If the tile filler never wins, **delete the routing rather than tune it**: drop
`CoverageFiller`, `AdaptiveFiller`, `TileComputeFiller`,
`shouldUseTileRasterizer`, and the `RasterizerMode` force modes that depend on
them. All four workaround sites in §1.2 then delete themselves, and
`paint/paint.go` loses the comment explaining a filler that no longer exists.
`AnalyticFiller` becomes the CPU rasterizer, singular, with the benchmark as the
reason. Another ~1,000 lines.

If it wins somewhere, keep the routing and narrow it to that region.

Stopping here is a legitimate outcome: the tree is smaller, the naming no longer
lies, four undocumented guards become one documented decision, and the
rasterizer choice is backed by measurement instead of by a comment.

## 8. Phase 3 — The strip generator, if the number says so

Only past the gate, with phase 1's numbers as the target.

1. Strip generator over the flattened segment list: per-scanline-band runs into
   a contiguous alpha buffer, no tile map, no dense per-fill backdrop array.
   Multi-contour winding correct in the first commit — that is the defect behind
   all four guards, and fixing it is what unlocks text.
2. Diff against `testdata/golden/vello-sparse-strips/`. Match the README's
   0.90%–3.80% band or explain the gap.
3. Diff against `AnalyticFiller` across the existing golden corpus. The CPU
   rasterizer is the reference the GPU path is already diffed against; the strip
   generator joins that contract rather than replacing it.
4. Only then, SIMD **over runs**. Wide operations on a contiguous alpha buffer
   are the entire point, and vectorizing the wrong unit is why the last attempt
   read as dead weight.

## 9. Phase 4 — Wire it in

Behind an explicit `RasterizerStrips` mode, off by default, golden corpus green.
Flip the default only when the phase 1 benchmark, rerun, shows the generator
beating `AnalyticFiller` on workloads that appear in real frames.
`text/glyph_mask_rasterizer.go` is the first switchover candidate: it is the
workload the guards currently forbid, and the one with a measurable cold-cache
win.

## 10. Phase 5 — GPU strip compositing

Last, and only after phase 4's default has flipped. CPU generates strips, GPU
composites them. `strip.wgsl` is the starting point and the naga fixture already
proves it compiles.

## Non-goals

- Reviving the compute pipeline. `SelectPipeline`'s measured table stands.
- Replacing `AnalyticFiller`. It stays as the reference the GPU path is diffed
  against, whatever wins on speed.
- Public API changes. There are none to make (§3).

## Risk

Phase 3 is novel work with no Go prior art, and its payoff on a GPU host is
bounded by two caching layers that already exist. Phases 0–2 are cheap, remove
code that actively misleads, and produce the number that says whether phase 3 is
worth starting. Do them regardless. Do not start phase 3 without that number,
and do not start it ahead of damage-rect upload unless the target is a
CPU-only host.
