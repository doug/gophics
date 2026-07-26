# Gossamer — a Flutter-class UI framework in pure Go

This is the planning document for gossamer: an idiomatic-Go reimagining of
Flutter — its architecture, not its API surface — built on a pure-Go GPU stack
with zero CGo, targeting desktop, web (WASM), and eventually mobile.

Status: **planning**. Nothing here is built yet.

## 1. Vision & principles

Flutter's core insight is a *pipeline*: immutable widget descriptions are
diffed into a retained element tree, which owns a render tree that does
single-pass constraint layout and paints into a compositable layer tree. That
pipeline — not Dart, not Material, not Skia — is what gossamer ports.

Principles, in priority order:

1. **`go build` is the whole story.** `CGO_ENABLED=0` everywhere in the core.
   One static binary per platform. This is the reason to build this in Go at
   all, and it is non-negotiable; any dependency that breaks it is out.
2. **Idiomatic Go, Flutter architecture.** Concepts transfer (widgets,
   constraints-down/sizes-up, keys, tickers); names and API shapes are
   designed for Go — interfaces and struct embedding instead of class
   hierarchies, struct literals instead of named parameters, generics where
   they pay for themselves.
3. **Testable without a display.** Every layer — layout, painting, gestures,
   full widget trees — must run headless in CI. Golden-image tests are a
   first-class feature from day one (the gogpu/wgpu software adapter makes
   this cheap).
4. **Honest scope.** Flutter is ~1.5M lines of framework Dart plus a C++
   engine plus years of text/a11y/IME grind. Gossamer succeeds by sequencing
   ruthlessly: prove the pipeline on desktop first, keep web close behind
   (it shares the renderer), and treat mobile and accessibility as designed-in
   but late-built.

### Non-goals (initially, some permanently)

- Dart source compatibility or mechanical transpilation of Flutter code.
- Pixel-fidelity clones of Material/Cupertino. Gossamer ships one clean
  default design language; Material-likeness is a theming exercise for later.
- Flutter's plugin ecosystem, platform channels, or embedding-into-native-app
  story.
- Hot reload parity with the Dart VM (see §6.3 for what replaces it).

### 1.1 The thesis — where gossamer beats Flutter

"Flutter but Go" loses to Flutter on every axis Flutter is good at. The
project is justified by the axes where Go *structurally* wins — these drive
prioritization throughout:

1. **A library, not an SDK.** Gossamer is a `go.mod` line. No `flutter
   doctor`, no channels, no engine artifacts; `go run` opens a window and
   `GOOS=windows go build` cross-compiles a static binary from a Mac. The
   Dart-VM/engine split makes this permanently impossible for Flutter.
2. **Embeddable, not app-owning.** Flutter must own `main()` and the
   lifecycle; add-to-app is famously painful. Gossamer serves programs
   Flutter can't: a CLI that pops a window, a server with a local admin UI,
   a pipeline with a live visualization pane.
3. **One language, infra-grade ecosystem.** The same binary embeds HTTP
   servers, pure-Go SQLite, gRPC; client and server share types. The
   natural gossamer app — tools, dashboards, local-first apps — sits on
   Go's home turf, where Dart needs FFI and platform channels.
4. **Real concurrency.** Goroutines with shared memory + the single-UI-
   goroutine rule (§4.6) + race detector, vs copy-based isolates.
   Streaming data into a live UI — Flutter's awkward case — is the easy case.
5. **Everything is `go test`.** Headless goldens, fuzzing, benchmarks,
   pprof, execution traces — ordinary tooling, no emulators or bespoke
   harnesses. And the offscreen path makes UI a *rendering library*:
   servers can render widget trees to images/reports headlessly.
6. **No codegen.** No build_runner/freezed/`.g.dart` layer between you and
   your program; structs, generics, stdlib.
7. **Distilled surface, Go-style stability.** ~a tenth of Flutter's
   accreted API, and the Go 1 compatibility ethos after 1.0 — no upgrade
   treadmill.

Corollary: gossamer does not chase Flutter's center of gravity (consumer
mobile). It wins first where Flutter is weakest — desktop tools, embedded
UIs inside larger Go programs, server-side rendering, single-binary
distribution — which is also why mobile is sequenced last (§6.4).

## 2. What "porting Flutter" actually means — scope inventory

| Flutter piece | What it is | Gossamer plan |
| --- | --- | --- |
| Engine (C++): Skia/Impeller | GPU rasterization, compositing | **Build**: vector renderer on gogpu/wgpu (§5) |
| Engine: text (SkParagraph/HarfBuzz/ICU) | shaping, bidi, line breaking, font discovery | **Adopt**: go-text/typesetting (incl. `fontscan` fallback/discovery) + build paragraph layout |
| Engine: image decoding (Skia codecs) | png/jpeg/webp/avif/… | **Adopt**: stdlib + x/image; gen2brain wazero/purego codecs for webp-encode/avif if needed |
| Engine: platform embedders | windows, input, vsync, IME | **Adopt/extend**: gophics windowing backends (Cocoa/X11/Win32/WASM) |
| `dart:ui` | Canvas, Paint, Path, Scene | **Build**: the `paint` + `scene` packages |
| foundation/ | keys, diagnostics, change notification | **Build** (small) |
| rendering/ | RenderObject protocol, boxes, flex, stack, viewport | **Build** — the heart of the port |
| widgets/ | Widget/Element trees, State, reconciliation, focus, routing | **Build** — the other heart |
| gestures/ | pointer routing, gesture arena | **Build** |
| animation/ | tickers, curves, controllers | **Build** (gophics `tween`/`easing` are a seed) |
| scheduler/ | frame pipeline, vsync phases | **Build** (small but load-bearing) |
| material/, cupertino/ | design languages | **Build one** default theme, late |
| semantics + a11y bridges | accessibility tree → OS APIs | **Design early, build late** (§6.5) |
| text editing + IME | TextField, composing regions, keyboards | **Build late**; hardest single feature (§6.1) |
| Tooling: hot reload, DevTools, widget inspector | DX | **Reimagine** for Go (§6.3) |
| dart:isolates / event loop | concurrency model | **Redesign**: goroutines + single UI thread (§4.6) |

## 3. Architecture

Same layered shape as Flutter, as Go packages (dependency arrows point down):

```
 widgets: material-ish theme, scrolling, text editing, routing
 ────────────────────────────────────────────────────────────
 widget:  Widget/Element trees, State, BuildContext, keys,
          focus, reconciler                     (≈ widgets/)
 gesture: hit testing, pointer routing, arena   (≈ gestures/)
 anim:    Ticker, curves, controllers           (≈ animation/)
 ────────────────────────────────────────────────────────────
 layout:  RenderObject protocol, Box model, flex, stack,
          viewports, sliver-equivalent          (≈ rendering/)
 ────────────────────────────────────────────────────────────
 scene:   retained layer tree, display lists, compositor (≈ dart:ui Scene)
 paint:   Canvas, Path, Paint, gradients, images, clips  (≈ dart:ui Canvas)
 text:    shaping (go-text/typesetting), paragraph layout, font mgmt
 ────────────────────────────────────────────────────────────
 gpu:     vector renderer on gogpu/wgpu; software path for headless
 shell:   per-platform embedders — window, vsync, input, IME, clipboard
          (desktop: Cocoa/X11/Win32 via goffi · web: WASM · mobile: later)
 geom:    Point, Size, Rect, RRect, Offset, Affine/Mat4, EdgeInsets
```

The `shell`+`gpu` layers are where gophics experience transfers directly:
pure-Go windowing over goffi, gogpu/wgpu for the device/swapchain/pipeline
plumbing, and its software adapter for headless rendering. Gossamer's new
GPU work is the *vector* layer — analytic anti-aliased path filling, clip
stacks, gradients, blurs/shadows — which gophics (flat-shaded ear-clip
tessellation) deliberately doesn't attempt.

### Frame pipeline (per vsync, mirroring Flutter's phases)

```
input events → gesture arena → callbacks (may mark widgets dirty)
→ animation tickers fire
→ build: rebuild dirty widgets, reconcile element tree
→ layout: flush dirty render objects (constraints down, sizes up)
→ paint: dirty render objects record display lists into layers
→ composite: layer tree → GPU command encoding → present
```

Each phase is independently instrumentable and testable; the frame budget
(16.6ms/8.3ms) is tracked per-phase from the first milestone onward.

## 4. The idiomatic reimagining — key API decisions

These are the decisions that make gossamer Go rather than transliterated
Dart. Each gets an ADR (`docs/adr/`) when finalized; sketches here show intent.

### 4.1 Widgets are struct values; struct literals replace named parameters

Dart's named/default parameters are Flutter's constructor ergonomics. Go's
struct literals with field names are a near-perfect substitute — this is the
single luckiest language mapping in the whole port:

```go
widget.Column{
    MainAxis: widget.MainAxisCenter,
    Children: []widget.Widget{
        widget.Text{Value: "Hello, gossamer", Style: theme.Headline},
        widget.Padding{
            All: 16,
            Child: widget.Button{
                OnTap: s.increment,
                Child: widget.Text{Value: fmt.Sprintf("count: %d", s.n)},
            },
        },
    },
}
```

Widgets are small immutable values implementing a `Widget` interface.
Zero values must be meaningful defaults (Go has no default parameters, so the
zero value *is* the default — this discipline pervades every widget design).

### 4.2 Interfaces + embedding replace the class hierarchy

Flutter: `StatelessWidget`/`StatefulWidget`/`RenderObjectWidget` subclass
`Widget`; `RenderBox` subclasses `RenderObject`. Gossamer: small interfaces
(`Widget`, `StatefulWidget`, `RenderObjectWidget`) with capability checks via
type assertion, and shared behavior via embedded structs
(`layout.BoxBase` embeds the parent-data/size/constraints plumbing the way
`RenderBox` inherits it). No `protected`, no template-method overriding —
where Flutter uses "override `performLayout`", gossamer uses an interface
method the embedding struct must provide.

### 4.3 State via generics

```go
type Counter struct{ Start int }

func (c Counter) CreateState() widget.State { return &counterState{} }

type counterState struct {
    widget.StateBase[Counter]   // provides W() Counter, Context(), SetState()
    n int
}

func (s *counterState) Build(ctx widget.BuildContext) widget.Widget { ... }
```

`StateBase[W]` gives typed access to the current widget config without casts.
`SetState` takes a closure (or is a plain method call + mark-dirty — ADR to
decide; Dart's closure form exists for analyzer reasons Go doesn't share).

### 4.4 Keys, BuildContext, inherited data

Keys port directly (interface + a few concrete types). `BuildContext` ports as
an interface over the element. `InheritedWidget` ports as a generics-flavored
`widget.Provide[T]` / `widget.Of[T](ctx)` pair — same O(1) lookup and
dependency registration as Flutter, better call-site types than
`context.dependOnInheritedWidgetOfExactType`.

### 4.5 Layout protocol ports intact

Constraints down, sizes up, parent sets position; one layout pass with
`relayoutBoundary` optimization. This protocol is Flutter's crown jewel and
is language-independent — gossamer keeps it exactly, including
`BoxConstraints` semantics, intrinsic sizing (with its cost caveats), and
baseline alignment. A sliver-equivalent protocol for scrolling comes later
(M6) but the door is designed in from the start (layout protocol is
per-render-object-type, not global).

### 4.6 Concurrency: goroutines, one UI owner

Dart is single-threaded with isolates; Go is the opposite. Gossamer's rule:
**the framework owns one goroutine (the UI loop); all tree mutation happens
there.** `gossamer.Post(func())` schedules work onto it (like
`SchedulerBinding`); any goroutine may Post. Long work runs in ordinary
goroutines and posts results back — this replaces both Dart's event loop and
`compute()`/isolates, and is *more* natural in Go than in Dart. Race detector
+ a debug-mode owner-check on every tree API keeps it honest.

### 4.7 Error handling

Build/layout/paint APIs do not return errors — a widget that can fail renders
an error box (Flutter's red screen), collected via a per-tree error handler.
Panics in user callbacks are recovered at phase boundaries in release mode,
fatal in debug mode. I/O-ish APIs (asset/image loading) return `error`
normally and have async widget wrappers (`widget.Future[T]`-style).

## 5. Rendering: the vector layer

The stack below (device, swapchain, shaders, windowing, WASM, software
rasterizer) exists — gogpu/wgpu + goffi, proven by gophics. What gossamer
must build is a **2D vector renderer** with UI-grade quality:

- analytic anti-aliasing (tessellate-and-MSAA is not good enough for text-adjacent UI edges; hairline strokes and rounded rects must be clean)
- even-odd/nonzero path fills, strokes with joins/caps/miters
- clip stack: rect / rrect / arbitrary path clips
- linear/radial/sweep gradients, image fills with sampling control
- blurs (backdrop + drop shadow) — needed early, every design language leans on shadows
- glyph rendering: alpha-texture atlas first; SDF/mesh glyphs later if scaling demands it
- layer compositing with opacity, transforms, and offscreen render targets

**Approach: two backends behind one `paint.Canvas` interface.**

1. **CPU rasterizer first** (M1): analytic-AA scanline filler (in the family
   of `x/image/vector`, extended with clips/gradients), blitting tiles to a
   GPU texture. Correct, testable, and fast enough for early milestones —
   this is roughly Flutter-on-CPU and desktop UIs survive it.
2. **GPU path renderer second** (M5): target Vello's **sparse-strips
   hybrid** architecture (CPU geometry/coverage in SIMD-friendly strips +
   lightweight GPU compositing), *not* the pure-compute piet-gpu pipeline.
   The field has spoken on this: Gio shipped a piet-gpu-derived compute
   renderer and **retired it in Jan 2025 as a failed experiment**
   (Pathfinder-style stencil/cover is what Gio runs today), and Linebender
   itself is betting on sparse strips (`vello_cpu`/`vello_hybrid`) over the
   compute pipeline. Sparse strips is also the most Go-portable design —
   far less compute-shader surface to trust, and it degrades gracefully to
   the pure-CPU backend (they share the strip generator). No Go port
   exists; this would be novel work, done *after* the framework exists so
   it has a full golden-test corpus and a reference backend to diff
   against. (Watch: Go's experimental `simd` package for the strip
   kernels; plain Go first, measured.)

The display-list format (`scene`) is designed for both from day one: paint
records commands, backends replay them. Damage tracking (repaint only dirty
regions) rides on the layer tree, as in Flutter.

### 5.1 Ecosystem facts & prior art (researched July 2026)

Load-bearing dependencies, with fallbacks:

| Dependency | Status | Fallback if it stalls |
| --- | --- | --- |
| **gogpu/wgpu** (pure-Go WebGPU impl, Vulkan/Metal/DX12 via goffi) | active; proven by gophics incl. WASM + software adapter | `oliverbestmann/webgpu` (the maintained CGo wgpu-native lineage; rajveermalviya's original is archived, cogentcore's defers to it) — works, but breaks the zero-CGo principle |
| **go-text/typesetting** | v0.3.x, active, shared Fyne/Gio governance | none realistic — this is *the* Go text stack; pin + vendor, private fork if it stalls |
| **purego** | active (Ebitengine-proven); Tier 1 incl. iOS/Android | CGo per-platform shims (principle #1 casualty) |
| **AccessKit** (`accesskit_c` via purego) | greenfield — no Go binding exists | pure-Go per-platform a11y bridges (much more work) |
| **gen2brain codecs** (wazero/purego webp/avif) | active, CGo-free | stdlib + x/image only (webp decode-only, no avif) |

Prior art to learn from (and where gossamer differs):

- **gogpu/ui + gogpu/gg** — the gogpu org's own toolkit stack (MIT,
  active, essentially one primary author with heavy AI assistance; very
  high velocity, unverified depth). gogpu/ui is retained-mode widgets +
  signals — a Qt/SwiftUI-style model, *not* Flutter's rebuild-and-
  reconcile, so it doesn't occupy gossamer's niche; its ADRs (damage
  tracking, layer tree, a11y role model) are worth reading regardless.
  **gogpu/gg is the strategic piece**: it claims an analytic-AA CPU
  rasterizer + tiered GPU pipeline (SDF/convex/stencil+cover/MSDF glyphs)
  — exactly gossamer's M1/M5 build items. M0 adds a hands-on evaluation
  spike: golden scenes (rrects/clips/gradients/shadows/blur) + perf vs
  the plan's own prototype. If gg's *rendering* holds up, M1 becomes
  "adopt gg as backend #1 behind `paint.Canvas`" and the sparse-strips
  work moves to opportunistic. Its *text shaping* (a from-scratch
  GSUB/GPOS engine, explicitly not go-text/typesetting) is where
  skepticism concentrated — and the spike **confirmed it empirically**
  (2026-07-24, gg v0.50.x, `paint/shaping_spike_test.go`): gg's shaper
  applies **zero positional substitution to Arabic** (identical glyph
  IDs to naive per-rune rendering, no bidi) — its "GSUB/GPOS support"
  covers Latin ligatures/kerning only. Two structural cautions also
  found: `Face.Advance` (measurement) bypasses the shaper `DrawString`
  uses — a latent layout/paint mismatch once shaping matters — and
  `Context.Image()` copies the full surface per call (web present-path
  cost). Verdict: gg is the *rendering* backend; text beyond Latin goes
  through go-text/typesetting (M7 text package), feeding gg positioned
  glyphs via `DrawShapedGlyphs`. **GPU accelerator spike (2026-07-25)**:
  gg's `gg/gpu` registration is process-global and has no CPU readback —
  offscreen `Image()` renders blank, breaking golden tests, Headless, and
  the web present path. Benchmarks under it are meaningless (work never
  rasterizes). Gated behind `-tags gossamer_gpu` as experimental;
  adoption blocked on per-context opt-in + readback upstream (or
  gossamer's own M5 backend). Hedge stands: v0.x weekly churn and
  bus-factor-one mean gg must stay *a* backend behind the Canvas
  interface, never a foundation.
- **Cogent Core** — the closest existing "Flutter-in-Go" in scope
  (retained widgets, Material 3, go-text stack). Its 2D UI is
  CPU-rasterized (rasterx) with GPU reserved for 3D; gossamer's bet on a
  GPU vector path and a Flutter-fidelity pipeline (element reconciliation,
  constraint protocol, layer compositing) is the differentiator. Small bus
  factor, pre-1.0 — study it, don't build on it.
- **Gio** — a decade of hard-won renderer and platform lessons, dual
  MIT/Unlicense (borrowable with attribution). Its compute-renderer
  retirement (§5) and its ops-based coupling are both cautionary inputs.
- **Ebitengine** — the existence proof for CGo-free macOS/Windows via
  purego and for shipping Go on mobile/web at scale; its windowing lives
  under `internal/`, so gossamer copies patterns, not packages.
- **go-flutter** — embedder-only (Dart still runs the UI), abandoned 2023;
  confirms nobody has actually done this port.

## 6. The hard problems — risk register

Ordered by how likely they are to kill or stall the project. Each gets a
cheap spike before its phase commits.

### 6.1 Text — the boss fight

Static text is in better shape than feared: go-text/typesetting (v0.3.x,
BSD-3, jointly governed by Fyne/Gio maintainers and used in production by
Fyne, Gio, and Ebitengine) covers shaping (full HarfBuzz port), bidi,
segmentation/line-break opportunities, variable fonts, **and** — via
`fontscan` — system font discovery and CJK/emoji fallback. Gossamer builds
paragraph layout (runs → bidi reorder → line break → align → styles) above
it. Main dependency caveat: pre-1.0 API churn.

Text **editing** is the trap: composing regions, IME integration
(Cocoa NSTextInputClient, Windows TSF, Linux ibus/fcitx, browser
composition events), selection/caret affinity across bidi boundaries,
keyboard shortcuts per platform. Flutter took years to make TextField good.
Plan: a deliberately-bounded `TextInput` (M7) — LTR-first, IME-correct on
macOS + web before the others, and honest documentation of what it can't do
yet. IME protocol is designed into the shell interface from M0 even though
implementation comes late.

### 6.2 Renderer quality/perf

Risk: the CPU rasterizer stalls at complexity (large blurs, many layers) or
the compute renderer stalls on gogpu/wgpu maturity (compute shader paths,
naga coverage). Mitigation: continuous benchmark suite (frame-time per phase
on reference scenes) from M1; the two-backend design means neither is a
single point of failure. Spike in M0: blur + clip-heavy scene through both a
prototype scanline filler and a gogpu compute shader, measured.

### 6.3 Developer experience without hot reload

Dart-VM hot reload is Flutter's signature DX and Go cannot replicate it.
Gossamer's answer, in order of leverage:

1. **Fast rebuild + state snapshot**: Go builds are seconds; a dev-mode
   harness restarts the process and restores serializable widget state
   (opt-in `StateSnapshot` interface) — "hot restart that remembers".
2. **Preview mode**: run a widget subtree headless, re-render on file save,
   view in a browser tab (the WASM/offscreen path makes this nearly free).
3. Live code swap is explicitly **out** early — yaegi (slow, incomplete
   module support, slowing maintenance) and `plugin` (no unload, exact
   toolchain match) are known tarpits. The one architecture worth a
   *future* spike is wazero-based widget-module swapping (recompile
   changed widget code to WASM, hot-swap the module) — plausible, shipped
   by no one. Revisit only if 1+2 prove insufficient.

This is a real, permanent DX gap vs Flutter and the plan says so honestly.

### 6.4 Mobile

Status 2026-07-25: Android runs (Pixel 7 AVD, live HN app: touch, fling,
navigation). Emulator frame pacing ~89ms avg under SwiftShader at
1080x2400 full-screen blits - the CPU present path is the smoothness
bottleneck; next levers are damage-rect blits through the bridge,
ANativeWindow direct presentation, or GPU present. Measure on real
hardware before optimizing further.


Mobile embedding (lifecycle, surfaces, touch, IME, app-store packaging) is
a whole platform team's worth of work in Flutter. Gossamer sequences it
last (M9): by then the shell interface has three desktop + one web
implementation and its shape is trustworthy. The whole Go ecosystem has
converged on one viable pattern — **Go as a library (`gomobile bind`-style
.aar/.xcframework) inside a thin native shell project**; gomobile's
whole-app mode is bitrotted and every framework (Gio's gogio, Fyne,
ebitenmobile) ships its own packaging tool on the bind pattern. Expect
mobile to be the one place CGo sneaks back in (purego is Tier-1 on
iOS/Android, but the bind machinery itself uses CGo — contain it in
`shell/`). Android first: NDK + Vulkan aligns with gogpu/wgpu, and its
lifecycle is more forgiving than iOS's. Until then, "runs on mobile" means
the WASM build in a mobile browser.

### 6.5 Accessibility

Not optional for a serious framework, and bolting it on later is how
frameworks end up with permanent a11y debt — the entire Go-native toolkit
ecosystem is evidence (Gio: none; Fyne: first pass only in v2.8, July 2026,
off by default; no Go AccessKit bindings exist at all). Plan: the semantics
tree (a parallel, cheap tree emitted during paint, as in Flutter) is in the
render-object protocol from M3, but the platform bridges land at M8. The
concrete route is a **purego binding to `accesskit_c`** (AccessKit covers
UIA/NSAccessibility/AT-SPI/Android from one tree API) — greenfield work
nobody has published, and plausibly gossamer's first upstream-able spinoff
package. Semantics correctness is testable headless (assert on the tree)
long before OS integration exists.

### 6.6 Go-specific risks

- **GC pauses**: Go 1.26's Green Tea GC (default since Feb 2026) keeps STW
  pauses well under 1ms and cut GC overhead 10–40% — fine for 120Hz frame
  budgets. The *real* risk is mutator assists stealing frame time under
  high allocation rates, so the rule is allocation discipline, not pause
  fear: zero allocations in layout/paint of unchanged subtrees;
  display-list and geometry pooling; benchmark-gated.
- **WASM**: no threads, larger binaries, GC + `syscall/js` overhead. Web
  stays a first-class CI target from M1 so regressions surface immediately
  (gophics proves the plumbing works).
- **Ecosystem bus factor**: gogpu/wgpu and go-text/typesetting are
  load-bearing external deps. Mitigation: pin versions, vendor if needed,
  contribute fixes upstream, keep the software rasterizer path as a floor.

## 7. Roadmap

Phases are sequential but overlap; each has a demo-able exit criterion —
if the exit demo can't ship, the phase isn't done. No calendar estimates
(solo-project variance makes them fiction); the ordering *is* the estimate:
every phase de-risks the ones after it.

- **M0 — Spikes & skeleton.** Repo layout, `geom` package, shell interface
  defined (window/vsync/input/IME/clipboard), gophics-derived desktop+WASM
  shells opening a gossamer-owned frame loop. Spikes: gogpu/gg evaluation
  (golden scenes, perf, shaping torture tests vs go-text/typesetting —
  §5.1); scanline-AA rasterizer proto (skipped if gg passes); go-text
  shaping proto; sparse-strip coverage proto on gogpu.
  *Exit: colored, vsynced, resizable window on macOS/Linux/Windows/web from
  one `main`; spike write-ups as ADRs.*
- **M1 — Canvas + CPU renderer.** `paint`/`scene` packages, display lists,
  layer tree, CPU backend, golden-test harness, frame-phase benchmarks.
  *Exit: static scene demo (rrects, gradients, clips, shadows, static
  shaped text) pixel-identical headless vs on-screen.*
- **M2 — Layout.** `layout` package: RenderObject protocol, box model, flex,
  stack, padding/align/constrained, relayout boundaries, hit-test geometry.
  No widgets yet — trees built by hand in tests.
  *Exit: layout test suite ported from Flutter's rendering tests (they
  translate almost mechanically and are a free conformance corpus).*
- **M3 — Widgets.** `widget` package: reconciler, State, keys, BuildContext,
  Provide/Of, error boundaries, semantics skeleton. The API-design ADRs of
  §4 get settled here in code.
  *Exit: counter + todo apps; reconciler fuzz tests (random tree mutations
  vs a naive rebuild oracle).*
- **M4 — Interaction & motion.** `gesture` (arena, tap/drag/hover/scroll),
  `anim` (tickers, curves, controllers, implicit animations), focus system,
  keyboard events.
  *Exit: draggable/animated demo at 120Hz with zero steady-state allocs;
  gesture arena test suite.*
- **M5 — GPU vector backend.** Vello-style compute renderer replacing the
  CPU blit path on capable targets; CPU backend remains as fallback +
  reference. *Exit: every golden test passes on both backends within
  tolerance; benchmark suite shows the GPU wins where it should.*
- **M6 — Scrolling & viewports.** Sliver-equivalent protocol, lazy lists,
  scroll physics (platform-correct feel), overscroll.
  *Exit: 100k-item lazy list, butter-smooth, on desktop + web.*
- **M7 — Text editing + theme.** TextInput per §6.1; the default design
  language (controls: button, checkbox, slider, switch, text field, menus,
  dialogs); routing/navigation.
  *Exit: a real small app (settings-panel class) built only from public API;
  IME-correct text entry on macOS + web.*
- **M8 — A11y + DX + polish.** Semantics bridges (§6.5), widget inspector
  (tree dump → browser view), preview mode + state-snapshot dev harness,
  docs site, API freeze review for a v0.1 tag.
  *Exit: VoiceOver reads the M7 app; v0.1 released.*
- **M9 — Mobile.** Android shell first, then iOS (§6.4).
  *Exit: M7 app in an APK with touch, IME, lifecycle handling.*

### 7.1 M-HN: the driving application (added 2026-07-25)

The concrete goal that sequences all remaining work: **a smooth
HackerNews client on desktop, web, and mobile** — the app class Flutter is
usually reached for. Remaining work, in build order:

1. ~~Async bridge~~ `Post()` onto the UI goroutine (§4.6, small).
2. **Scroll physics**: velocity-tracked fling with friction decay —
   "smooth" lives or dies here.
3. **Lazy lists**: mount only the visible slice, measured-height cache
   (real M6 content; a 500-comment thread must be O(visible)).
4. **Images**: Canvas/scene op + widget.
5. **The app itself** (desktop/web first): feed → comments, real API.
6. **Rich text spans**: inline links, styles, selection (comments).
7. **Navigator**: route stack, transitions, back handling.
8. **Overlays/Stack**, theming via Provide/Of, error boundaries,
   multi-line editing, pointer cursors.
9. **M9 mobile shells**: Android first (SDK + emulator present on the
   dev machine), `gomobile bind`-style embedding in a thin native shell,
   touch + on-screen keyboard + lifecycle + safe areas; then iOS (Xcode
   present). Exit: the HN app on a phone at 60fps+, IME-correct.
10. A11y bridges + the M5 GPU backend ride behind, unblocked by any of
    the above.

## 8. Testing strategy

- **Golden images** everywhere, rendered headless via the software path;
  per-backend tolerance for GPU. (gophics's `RenderToImage`-in-CI pattern,
  industrialized.)
- **Flutter's own test suites as conformance corpus** — rendering/ and
  widgets/ tests encode a decade of edge cases; port them alongside each
  phase (they're BSD-licensed; attribute clearly).
- **Fuzzing**: reconciler (random mutations vs rebuild oracle), layout
  (random constraints — invariant: size always satisfies constraints),
  text (random unicode through the paragraph layouter).
- **Benchmarks as tests**: frame-phase budgets and steady-state allocation
  counts asserted in CI, not just graphed.
- **Race detector** on the full suite; debug-mode UI-goroutine ownership
  asserts.

## 9. Repo layout

```
gossamer/
  geom/          # Point, Size, Rect, RRect, Mat4, EdgeInsets
  paint/         # Canvas, Path, Paint, Color, gradients, images
  text/          # shaping wrapper, paragraph layout, fonts
  scene/         # display lists, layer tree, damage tracking
  gpu/           # backends: cpu/ (scanline AA), wgpu/ (compute)
  shell/         # interface + desktop/ web/ (later android/ ios/)
  layout/        # RenderObject protocol + box/flex/stack/viewport
  widget/        # Widget/Element, State, keys, focus, reconciler
  gesture/       # hit testing, arena, recognizers
  anim/          # tickers, curves, controllers
  theme/         # the default design language (M7)
  app/           # top-level runner tying shell→scene→widget (gossamer.Run)
  internal/      # ffi plumbing shared by shells
  examples/
  docs/adr/      # one file per §4/§5/§6 decision as it's made
  bench/
```

## 10. Working method

- Every §4–§6 decision becomes a short ADR in `docs/adr/` when settled —
  the sketches above are proposals, not commitments.
- Flutter's source is the reference implementation: when in doubt about
  *behavior* (layout edge cases, gesture arena rules), match Flutter and
  cite the file in a comment; when in doubt about *API*, design for Go.
- Dependencies are replaceable, not relationships. Pin and vendor the
  load-bearing deps; send a fix upstream only when it's trivial. If a dep
  stalls or drifts, fork it privately or regenerate the needed subset
  behind the owning interface — no coordination overhead, no waiting on
  other projects' roadmaps. (Exception by necessity: go-text/typesetting's
  shaping correctness is not regenerable on demand; it gets pinned +
  vendored and treated as slow-moving infrastructure.)
