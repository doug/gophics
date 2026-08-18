# Audit: internal/gfx

A pass over the vendored graphics substrate (2026-08-17), looking for things
that would hurt a user rather than things that offend a linter.

`internal/gfx` is 75 packages of maintained forks of the gogpu lineage
(THIRD_PARTY.md). It is not gophics code, and churning it has a cost: every
local change is a future merge conflict. So the bar for touching it here is
"this can crash or silently corrupt a user's app", not "this could be tidier".

---

## Method

Counting markers is not an audit — 242 of the tree's TODOs are upstream feature
notes, and reading them tells you what gogpu's authors planned, not what is
broken. The sweeps that found things were:

- entry points called through `syscall.SyscallN` without a null guard
- `panic` in library (non-test) code
- errors assigned to `_`
- packages with no test files
- test files behind build tags that nothing runs

---

## 1. GL entry points were never validated — **fixed**

**Severity: high. A crash on real hardware, already observed.**

`Context.Load` assigns 126 function pointers from `getProcAddress` and checked
none of them. A name the driver does not export silently became 0 and `Load`
returned success. The failure then arrived at the first call as an access
violation with `PC=0` and an empty stack.

This is not theoretical: a gophics app died this way the first time it ran in a
UTM VM, and it took a crash dump to see that `glGenVertexArrays` was null.
Windows hands back a "GDI Generic" OpenGL 1.1 context whenever no graphics
driver is present — a VM, an RDP session, a fresh install.

Two fixes landed. The adapter now refuses a context below GL 3.3 / ES 3.0
(`AdapterCapabilities.Usable`), and `Load` now reports which required entry
points are missing. The second matters more: the first only catches a context
that *admits* to being old, and the nastier case is a driver advertising 3.3
that still fails to resolve a name — which GDI Generic and Mesa's d3d12 gallium
both do.

On the same VM the result is now a named diagnosis and a clean fallback:

    GL load: gl: 17 required entry points missing: glCreateShader, ...
    adapter selected name="Software Renderer" backend=Empty type=CPU

**Residual:** 103 GL call sites, of which 27 carry their own null guard. The
required set is now checked at load, so the remaining unguarded ones are
version- and extension-dependent functions. Those are correctly optional, but
each is a `PC=0` crash if reached on a driver lacking it. Worth a follow-up
sweep classifying them; not urgent, because the required floor is enforced.

---

## 2. A library that panics

**Severity: medium. Kills the host application.**

26 `panic` sites in non-test code. Most are in SPIR-V ray-tracing codegen, a
path gophics never takes, and they panic on internal invariants — defensible.

Two are not:

```go
func (d *Device) PushErrorScope(filter ErrorFilter) {
    panic("wgpu: browser PushErrorScope not yet implemented (Phase 2)")
}
```

`PushErrorScope` and `PopErrorScope` are exported methods on `Device` that
hard-kill the process on web. Nothing in gophics calls them today, so this is
latent rather than active — but "not yet implemented" is exactly the case that
should degrade, not abort. A no-op with a documented limitation costs a user
nothing; a panic costs them their app.

**Fixed.** Both are now no-ops, and `PopErrorScope` returns nil — honest,
since nothing was capturing. Scoped error capture stays unavailable on web,
which the browser console already covers. 26 panics down to 24; the rest are
internal invariants in codegen paths gophics does not take.

---

## 3. Ignored errors — mostly fine, and worth saying why

478 assignments to `_`. That number sounds alarming and mostly is not:

| Count | Call | Assessment |
|---|---|---|
| 134 | `MsgSend` | Objective-C sends; the error is rarely actionable |
| 65 | `source.Close` | closing after a read — conventional |
| 63 | `pass.End` | **worth revisiting** |
| 60 | `dc.Fill` | drawing ops on an in-memory context |
| 51 | `img.SetRGBA` | in-memory image writes |

`pass.End` is the one I would look at. A render pass failing to end is not a
cosmetic error — it can mean the encoder is in a bad state, and swallowing it
turns a diagnosable failure into a blank frame. Not changed here, because it
needs a judgement about what the caller could usefully do, which is a design
question for the substrate's owner rather than an audit fix.

---

## 4. Test coverage is better than it looks

63 of 75 packages carry tests. The apparent gaps are mostly an artefact of
asking on macOS: `wgpu/hal/gles/gl` reports no test files there because its
sources are `windows || linux`, and it does have tests.

Genuine gaps worth noting: `gg/raster`, `gg/gpu`, `gogpu/gpu/backend/native`
and `gogpu/internal/thread`. `internal/thread` is the interesting one — it
handles main-thread marshalling, where a bug is a deadlock rather than a wrong
pixel, and deadlocks are the hardest class to diagnose from a user report.

---

## 5. Tests that existed but never ran — **fixed**

Not strictly `internal/gfx`, but found while auditing it, and the same failure
mode as the substrate's silent entry points.

Six test files behind the `gophics_gpu` tag — GPU-vs-CPU equivalence, gradient
interpolation, opacity groups, shadows, backdrop blur, readback — were run by
no job. They are not rotten: all fourteen pass on Metal with zero skips. They
were simply invisible, and a GPU-vs-CPU divergence is precisely the regression
that reaches a user as a visual bug rather than a red build. CI now runs them.

Likewise `-tags nogpu` was compiled and never tested, despite being the path a
driverless machine actually lands on.

---

## Recommended next, in order

1. **Classify the 76 unguarded GL call sites** into core-3.3 (safe, covered by
   the load check) and extension-dependent (needs a guard). Mechanical, and it
   closes the crash class rather than its worst instance.
2. **Test `gogpu/internal/thread`**, where a bug is a deadlock.
3. **Revisit `pass.End`** error handling with whoever owns the render path.

Everything else found here is upstream's to prioritise, and is better raised
with the fork than patched locally.

---

## Addendum: code that looks alive (found during M10–M12)

Three separate times, work started against code that turned out never to run.
Recorded together because the pattern cost more than any individual instance,
and because each one looked entirely healthy from the outside.

- **`PipelineCache` and its nine `Stub*ID` types** (`internal/gpu/pipeline.go`)
  are unreachable. `PipelineCache` is held by `GPUSceneRenderer`, which is
  built by `Backend.RenderScene`, and `gpu.NewBackend()` is called nowhere in
  production — only in a doc comment. M10 was written to "replace
  `createStripPipeline`'s stub with a real compute pipeline"; doing so would
  have produced a working pipeline that nothing calls, and the tests asserting
  `GetStripPipeline() != 0` would have passed either way.

  **It cannot simply be deleted, and that is worth knowing before trying.**
  Removing `backend.go`, `renderer.go` and `pipeline.go` fails to compile:
  `memory.go`, `atlas.go`, `commands.go` and `command_encoder.go` all reference
  `Backend` or the `Stub*ID` types. Removing those in turn fails on
  `gpu_texture.go` (`MemoryManager`, `Backend`) and — decisively —
  `text_pipeline.go`, which uses `RenderPass` from the same cluster and is very
  much live: it draws glyphs. So the stub abstraction layer is not a severable
  dead subtree. It is entangled with working code, and disentangling it is a
  refactor rather than a deletion.

- **Three embedded shaders are never compiled into a pipeline**: `blend.wgsl`,
  `strip.wgsl` and `composite.wgsl`. Each has a `Get…ShaderSource()` accessor,
  and nothing calls any of them. `strip.wgsl` is the shader the sparse-strips
  milestone was named after. `composite.wgsl` matters for a second reason: it
  is the only render-stage shader with a runtime-sized array, so it is the sole
  reason the render path needs `_mslBufferSizes` wiring at all — wiring that
  therefore cannot be exercised.

- **`GPUFineRasterizer` compiled its shader, built its layouts and created three
  compute pipelines — then computed coverage in a Go loop.** It returned
  correct pixels, so every test passed. Only the *source* of the pixels was
  wrong.

What connects them is that none is detectable by reading the code that contains
them; each needs a question asked from outside — who constructs this, who calls
that accessor, where does this result actually come from. The defences that
worked were the ones that assert on an outcome rather than a return value: a
test that demands a real device, a diff against an independent implementation,
a benchmark with a second contestant.

**Not deleted here.** Removal is right for at least `PipelineCache`, but the
entanglement above makes it a refactor with a real chance of breaking glyph
rendering, and M12 is where the sparse-strips direction is being settled.
Deleting `strip.wgsl` while that is open would foreclose it by accident.

The three dead shaders are the cheap part: `blend.wgsl`, `strip.wgsl` and
`composite.wgsl` have no dependents at all beyond their own accessors, so those
come out whenever the direction is settled.
