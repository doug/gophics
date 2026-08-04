# Consolidating the forked substrate into the gophics repo — plan

**Status: DONE (2026-08-04).** All seven forks are vendored into the gophics
module under `internal/gfx/{gputypes,gpucontext,naga,wgpu,gogpu,gg}` and
`internal/audio`; imports rewritten; `go.mod` collapsed (no `doug/*`, no
`replace`); `~/src/go.work` retired (→ `go.work.retired`). Verified: standalone
`go build ./...` (no workspace), zero CGo, framework + GPU opacity/parity tests
pass, `nogpu` + `GOOS=js/wasm` build, and **`gomobile bind` builds the APK with
no replace directives** — confirming this also fixes the gomobile-ignores-go.work
trap. Licenses preserved per subtree + `THIRD_PARTY.md`. The forks remain intact
at `../third_party/*` (copied, not moved) and can be archived/deleted. Supersedes
the `deps-namespace-strategy` decision (separate tagged repos). Plan retained
below.

**Full-suite triage (`go test ./...`):** 100 packages pass; **0 failures caused
by the move.** The only 3 failing packages are pre-existing/environmental and
reproduce identically in `third_party/*` (byte-identical source): `gg/internal/gpu`
(3× `TestMetalStencil*` — software-backend MSAA), `naga/msl/internal/codegen`
(6× — `xcrun metal` toolchain not working on this host), and
`gogpu/internal/platform/darwin` (`TestDarwinAppRunSmoke` — opens a native Cocoa
window and blocks in the event loop, times out headless). All test files compile
under the new paths. For a green `go test ./...`, scope/skip these substrate
env-dependent tests (they need a real GPU / working `xcrun metal` / an interactive
display) — the plan's "scope substrate tests in CI" note.

## The ask

Stop maintaining seven separate forked repositories — `audio`, `gg`, `gogpu`,
`gpucontext`, `gputypes`, `naga`, `wgpu` — and instead bring them **into this
repository as packages**. This doc is what that looks like, how it's structured,
and why.

## What we're consolidating (measured, not guessed)

| Fork (`github.com/doug/…`) | Role | Non-test LOC | Pkgs | CGo | License |
|---|---|---:|---:|---|---|
| `wgpu` | pure-Go WebGPU impl (Vulkan/Metal/DX12 via goffi) | 133k | 41 | 0 | MIT |
| `gg` | 2D vector renderer (gophics's `paint` backend) | 128k | 59 | 0 | MIT |
| `naga` | WGSL → SPIR-V/MSL shader translator | 121k | 32 | 0 | MIT |
| `gogpu` | desktop windowing + higher-level GPU/creative-coding | 51k | 35 | 0 | MIT |
| `gputypes` | shared GPU enums/structs | 3.4k | 1 | 0 | MIT |
| `gpucontext` | opaque GPU handle types | 2.2k | 1 | 0 | MIT |
| `audio` | pure-Go audio output drivers (CoreAudio/PulseAudio/WASAPI/WebAudio) | 2.3k | 4 | 0 | MIT |
| **total** | | **~442k** | **173** | **0** | MIT |

(~830k LOC including tests.) Every one is **zero-CGo** — the single-binary /
cross-compile property that is the whole point of the stack. All MIT, all the
user's own forks of the `github.com/gogpu/*` lineage.

### The dependency graph is a clean DAG

Verified against `go list -deps`, not go.mod guesses. Non-test, non-example
imports only:

```
gputypes        (leaf)
naga            (leaf — no deps at all)
gpucontext  → gputypes
wgpu        → gpucontext, gputypes, naga           + go-webgpu/{goffi,webgpu}
gogpu       → gpucontext, gputypes, wgpu           + go-webgpu/{goffi,webgpu}
gg          → gpucontext, gputypes, naga, wgpu     + go-webgpu/{goffi,webgpu}, x/image, x/text
audio       → (independent)                         + go-webgpu/goffi, x/sys
```

No cycles. The two edges that looked like cycles are false: `gg → gogpu` exists
only in `gg/examples/*` (droppable), and `wgpu → gogpu` is a **comment URL**
(`// See: https://github.com/doug/gogpu/issues/98`), not an import. So the whole
set collapses into one module without breaking Go's package-cycle rule.

### The external boundary (what stays a dependency)

These are **not** forks and stay external — the bottom of the stack:

- `github.com/go-webgpu/goffi` — the dlopen/FFI layer (purego-class).
- `github.com/go-webgpu/webgpu` — low-level WebGPU binding.
- `golang.org/x/{image,sys,text,mobile}` — Go team, BSD, stdlib-adjacent.
- `github.com/go-text/typesetting` — HarfBuzz-class shaping.
- `github.com/{hajimehoshi/go-mp3, jfreymuth/oggvorbis}` — audio decoders.

So even after consolidation, gophics still pulls `go-webgpu/{goffi,webgpu}` —
that's the FFI floor the whole thing sits on, and it isn't ours to own.

## Decision: one module, substrate under `internal/`

Collapse all seven into the existing `github.com/doug/gophics` module as
internal packages. Import paths change once, globally; then there is one repo,
one `go.mod`, one `go build`.

### Proposed layout

```
github.com/doug/gophics/                 (one module, one go.mod)

  # ── the framework (unchanged, stays top-level) ──
  geom/ paint/ scene/ layout/ widget/ gesture/ anim/ text/ theme/
  app/ shell/ input/ sound/ chart/ cmd/ examples/ docs/ …

  # ── the vendored substrate (was 7 repos) ──
  internal/
    gfx/                        # graphics substrate root
      gputypes/                 ← github.com/doug/gputypes
      gpucontext/               ← github.com/doug/gpucontext
      naga/                     ← github.com/doug/naga
      wgpu/                     ← github.com/doug/wgpu
      gogpu/                    ← github.com/doug/gogpu
      gg/                       ← github.com/doug/gg   (incl. gg/gpu, gg/text, gg/integration/ggcanvas)
    audio/                      ← github.com/doug/audio   (paired with the existing sound/ package)
```

Import-path rewrite (a single mechanical global replace, unambiguous because only
the `doug/*` namespace is forked):

```
github.com/doug/gputypes    → github.com/doug/gophics/internal/gfx/gputypes
github.com/doug/gpucontext  → github.com/doug/gophics/internal/gfx/gpucontext
github.com/doug/naga        → github.com/doug/gophics/internal/gfx/naga
github.com/doug/wgpu        → github.com/doug/gophics/internal/gfx/wgpu
github.com/doug/gogpu       → github.com/doug/gophics/internal/gfx/gogpu
github.com/doug/gg          → github.com/doug/gophics/internal/gfx/gg
github.com/doug/audio       → github.com/doug/gophics/internal/audio
```

Naming note: the substrate root is `internal/gfx/` (not `internal/gpu/`) so gg's
own `internal/gpu` package doesn't read as `internal/gpu/gg/internal/gpu`. Nested
`internal/` is legal and works; `gfx` just keeps it legible.

### Why `internal/` rather than public `gophics/gpu/…`

`internal/` is the honest signal: **this substrate is gophics's private plumbing,
not a public API we promise to keep stable.** Go enforces it — nothing outside
`github.com/doug/gophics/…` can import it, so we keep full freedom to refactor
gg/wgpu without semver obligations to anyone.

The one thing to check first (a bounded audit, already scoped): a few *exported*
gophics signatures currently name substrate types, e.g.
`paint.Painter.GPUCanvas(dc *gg.Context)`, and `shell/*` `RenderGPU(func(*gg.Context))`.
Almost all are unexported functions or methods on **unexported** types, so no
external caller can name them anyway. The audit is:

- Grep exported gophics API for `gg.`/`gogpu.`/`wgpu.`/`gpucontext.`/`gputypes.`.
- For each real leak (only `paint.Painter.GPUCanvas` is a genuinely-exported one),
  either unexport it or accept it (an exported method that names an internal type
  is legal — it's simply only constructible from inside the module, which is
  already true today since you can't get a `*gg.Context` without depending on gg).
- Fix any gophics **example** that imports a fork directly (examples are ours; a
  handful may reference `gg`/`gogpu`).

If the audit turns up a substrate type we genuinely want in the public API, the
fallback is to host *only that one* package publicly (e.g. keep `gg` at
`gophics/gpu/gg`) and everything else under `internal/`. Not expected to be
needed.

## Why do this (the payoff)

1. **Atomic cross-stack changes.** The GPU opacity-layer work just done spanned
   `gg` **and** gophics, and to ship it required editing a separate repo, a
   `go.work` for desktop, **and** temporary `replace` directives in `go.mod` for
   the device build. In one module that's a single commit, no version dance.
2. **Kills the `gomobile`-ignores-`go.work` trap outright.** `gomobile bind` does
   not honor `go.work`, so device builds silently froze stale `gg` unless you hand-
   added `replace` directives (a live footgun — a device test could pass against
   code that isn't the code you edited). With everything in one module there are
   **no external `doug/*` modules**, so nothing to `replace`; device builds just
   work.
3. **One repo, not seven.** No per-fork tagging, no "which `gg` tag does gophics
   pin," no seven-way version bookkeeping, no `go.work` at all.
4. **Matches the working philosophy.** These are permanent forks; upstream `gogpu`
   sync is already abandoned (regenerate/own, don't coordinate). Separateness was
   buying an upstream-merge path we don't use.
5. **Reproducible by construction.** `git clone` gives the exact substrate; no
   chance of a consumer resolving a different `doug/gg` tag than intended.

## What we give up (honest tradeoffs)

- **Repo size.** gophics's own code is a small fraction of a ~442k-LOC
  substrate + shader compiler. Mitigated by the `internal/gfx/` subtree isolation
  and a README that says "the framework is the top-level packages; `internal/gfx`
  is the vendored GPU substrate."
- **`go test ./...` gets heavy.** It now includes the substrate suite. Mitigate by
  scoping CI: a fast `go test ./<framework-pkgs>/...` gate, and the substrate suite
  behind its own job (or a build tag) run less often. The substrate rarely changes
  except when we change it — and when we do, we *want* its tests.
- **No independent publishability.** `wgpu`/`gg`/`naga` can no longer be imported
  standalone by other projects. Per the stated philosophy this was never the goal;
  if it ever becomes one, a package can be lifted back out (the git history and
  MIT license travel with it).
- **Upstream sync is fully manual** (already effectively true).

## Migration plan (ordered, mechanical)

1. **Snapshot import.** For each fork, copy its tree into the target subtree,
   dropping `go.mod`, `go.sum`, `.git`, CI configs, and `examples/` (gophics has
   its own; this also removes the `gg → gogpu` example edge). Optionally use
   `git subtree add`/`git read-tree` if commit history is wanted — not required.
2. **Global import rewrite.** Apply the seven path rewrites above across all `.go`
   files (both the moved substrate and gophics's own source) with a scripted
   `gofmt -r` / `sed` + `goimports` pass. Unambiguous — only `doug/*` is forked.
3. **Collapse `go.mod`.** Remove the seven `require github.com/doug/*` and the
   seven temporary `replace` lines; keep/add the external deps the forks pulled
   (`go-webgpu/{goffi,webgpu}` are already indirect; `x/{image,sys,text}` already
   present). Run `go mod tidy`.
4. **Delete `~/src/go.work`.** With no external `doug/*` modules it's dead weight,
   and its absence is what makes gomobile correct.
5. **Preserve licenses.** Keep each fork's `LICENSE` file in its subtree directory
   (MIT requires retaining the copyright notice) and add a top-level
   `THIRD_PARTY.md` crediting the `gogpu/*` originals.
6. **API-leak audit** (above): fix the ≤handful of exported signatures / examples
   that name substrate types.
7. **Green the build across all targets:** `go build ./...`, `go test` (framework
   + gpu-tagged), `GOOS=js/wasm`, `GOOS=android`/`ios` via `gophics run`, and
   `-tags nogpu`. The opacity tests + render-matrix parity are the regression
   guard.
8. **CI import denylist.** Keep the rule that first-party framework packages
   (`widget`, `paint`, `chart`, …) import only stdlib + gophics's own layers +
   `internal/gfx` — not random new external deps.

## Verification it's still "one static binary, zero CGo"

Post-migration, re-assert the core invariant: `grep -rl 'import "C"'` across the
whole module returns nothing, and `CGO_ENABLED=0 go build ./...` succeeds for
darwin/linux/windows/js. (Mobile `bind` is the one contained CGo exception, in
`shell/mobile`, unchanged by this.)

## Rollback

Each subtree is self-contained with its `LICENSE`; if consolidation ever needs
reversing, a package lifts back out to its own repo by reverse-applying the path
rewrite. Low-risk, and the change is a mechanical rewrite rather than a
behavioral one — the opacity/parity/golden suites prove behavior is unchanged.
