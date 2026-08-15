# Gophics — a cross-platform UI library for Go

Gophics is a UI library for Go: it draws every pixel itself on a pure-Go GPU
stack with zero CGo, so one codebase renders the same interface on desktop,
web (WASM), iOS, Android, and a terminal.

This document is the architecture rationale — the *why* behind the pipeline
and the decisions that shaped the API. It began as a plan and is kept as
reasoning. **Status: built.** The pipeline described here works on all listed
targets; §7 tracks what is done and what is not. Read this for design
reasoning, `docs/rsc-readiness-review.md` for an outside critique, and the git
history for the current state.

A note on lineage, stated once: the rendering pipeline here — immutable widget
descriptions diffed into a retained element tree, which owns a render tree
doing single-pass constraint layout and painting into a compositable layer
tree — is the design Flutter proved at scale, and gophics adopts it because it
is the best-understood architecture for this problem. The reference is
architectural only. Gophics is not a port, not a compatibility layer, and not
a competitor positioned against anything; it is a Go library, designed for Go,
that happens to share a pipeline shape with prior art. Where behavior is
subtle (layout edge cases, gesture disambiguation), matching a well-tested
implementation is a shortcut to correctness, and the code cites it.

## 1. Principles

In priority order. These are tie-breakers, not slogans — each has rejected a
dependency or a design at some point.

1. **`go build` is the whole story.** `CGO_ENABLED=0` everywhere in the core.
   One static binary per platform. This is the reason to build this in Go at
   all, and it is non-negotiable; any dependency that breaks it is out.
2. **Idiomatic Go.** Interfaces and struct embedding instead of class
   hierarchies, struct literals instead of named parameters, generics where
   they pay for themselves. Zero values are meaningful defaults.
3. **Testable without a display.** Every layer — layout, painting, gestures,
   full widget trees, accessibility semantics — runs headless in CI.
   Golden-image tests are a first-class feature (the software adapter makes
   this cheap).
4. **Honest scope.** A UI toolkit is years of text, accessibility and IME
   grind. Gophics succeeds by sequencing ruthlessly and by documenting what
   it cannot yet do, rather than by implying completeness.

### 1.1 Non-goals

- Pixel-fidelity clones of Material or Cupertino. Gophics ships one clean
  default design language; imitating another is a theming exercise.
- A plugin ecosystem or platform-channel architecture. Platform services are
  plain `ctx.<Cap>()` calls that return nil where a host doesn't offer them.
- Owning `main()`. Gophics is a library a program calls, not a runtime that
  calls the program.
- Hot reload in the dynamic-VM sense (§6.3 covers what replaces it).

### 1.2 What the library is for

These properties follow from being a Go library rather than an SDK, and they
drive prioritization throughout:

1. **A library, not a platform.** Gophics is a `go.mod` line. No doctor
   command, no engine artifacts, no channels; `go run` opens a window and
   `GOOS=windows go build` cross-compiles a static binary from a Mac.
2. **Embeddable.** It does not need to own the process. The programs it
   serves well include ones a UI framework usually can't: a CLI that pops a
   window, a server with a local admin UI, a data pipeline with a live
   visualization pane.
3. **One language, infra-grade ecosystem.** The same binary embeds HTTP
   servers, pure-Go SQLite, gRPC; client and server share types. The natural
   gophics app — tools, dashboards, local-first apps — sits on Go's home
   turf.
4. **Real concurrency.** Goroutines with shared memory, the single-UI-goroutine
   rule (§4.6), and the race detector. Streaming live data into a UI is the
   easy case here, not the awkward one.
5. **Everything is `go test`.** Headless goldens, fuzzing, benchmarks, pprof,
   execution traces — ordinary tooling, no emulators or bespoke harnesses.
   The offscreen path also makes the UI a *rendering library*: a server can
   render widget trees to images and reports with no display at all.
6. **No codegen.** No generated-source layer between you and your program;
   structs, generics, stdlib.
7. **Small surface, Go-style stability.** A distilled API and the Go 1
   compatibility ethos after 1.0 — no upgrade treadmill.

The corollary is a center of gravity: desktop tools, UIs embedded inside
larger Go programs, server-side rendering, single-binary distribution. Mobile
is supported and was sequenced late (§6.4) because that ordering de-risked
everything before it.

## 2. Scope inventory

What a toolkit of this kind needs, and how gophics gets it. "Build" means
gophics owns the code; "adopt" means a dependency does the work.

| Subsystem | What it is | Approach | State |
| --- | --- | --- | --- |
| GPU rasterization, compositing | vector renderer on wgpu | **Build** (§5) | working; GPU vector backend still ahead |
| Text shaping, bidi, line breaking, font discovery | HarfBuzz-class stack | **Adopt** go-text/typesetting + `fontscan` | working, incl. paragraph layout built above it |
| Image decoding | png/jpeg/webp/… | **Adopt** stdlib + x/image | working |
| Platform embedding | windows, input, vsync, IME | **Adopt/extend** the windowing layer | working on 4 desktop/web targets + mobile |
| Canvas, Paint, Path, Scene | immediate + retained drawing | **Build** — `paint`, `scene` | working |
| Render objects, box model, flex, stack, viewport | layout protocol | **Build** — `layout` | working |
| Widget/element trees, State, reconciliation, focus, routing | the declarative layer | **Build** — `widget` | working |
| Pointer routing, gesture disambiguation | gestures | **Build** — in `app`/`widget` | working |
| Tickers, curves, controllers | animation | **Build** — `anim` | working |
| Frame pipeline, vsync phases | scheduling | **Build** — `app` | working |
| Default design language | theme + controls | **Build** — `theme` | working |
| Semantics + platform a11y bridges | accessibility | **Build** (§6.5) | semantics + Android, iOS, web, macOS done; Linux and Windows pending |
| Text editing + IME | TextField, composing regions | **Build** (§6.1) | working incl. IME preedit; single-line |
| Internationalization | number/date/money formats, bidi, RTL layout | **Build** — `intl` + `widget` | formats and RTL done; message catalog pending |
| Inspector, preview, dev harness | DX | **Build** (§6.3) | state-snapshot restart + inspector done |

## 3. Architecture

Layered Go packages, dependency arrows pointing down. The graph is acyclic
and machine-checked, as is the zero-CGo invariant.

```
 theme:   default design language — controls, dialogs, pickers, tables
 chart:   data visualization built on the same primitives
 ────────────────────────────────────────────────────────────
 widget:  Widget/Element trees, State, Ctx, keys, focus,
          reconciler, gestures, drag & drop, navigation
 anim:    Ticker, curves, controllers
 ────────────────────────────────────────────────────────────
 layout:  render-object protocol, box model, flex, stack,
          viewports, semantics collection
 ────────────────────────────────────────────────────────────
 scene:   retained layer tree, display lists, damage tracking
 paint:   Canvas, Path, Paint, gradients, images, clips
 text:    shaping, bidi, paragraph layout, font management
 ────────────────────────────────────────────────────────────
 shell:   per-platform embedding — window, vsync, input, IME,
          clipboard, and ~20 optional capabilities
          (desktop · web · mobile · terminal)
 sound:   lock-free pull-model audio mixer
 intl:    locale-aware number, money and date formatting
 input:   device-independent input types
 geom:    Point, Size, Rect, Insets, transforms
 ────────────────────────────────────────────────────────────
 app:     the runner tying shell → scene → widget together,
          plus Headless for display-free rendering and tests
 internal/gfx: the vendored GPU substrate (wgpu, naga, windowing)
```

`app` sits at the bottom of that listing because it depends on everything
above it — it is the composition root, not a foundation.

### Frame pipeline (per vsync)

```
input events → gesture routing → callbacks (may mark widgets dirty)
→ posted work from other goroutines drains
→ animation tickers fire
→ build: rebuild dirty widgets, reconcile element tree
→ layout: flush dirty render objects (constraints down, sizes up)
→ paint: dirty render objects record display lists into layers
→ diff: compare against last frame, compute damage
→ composite: layer tree → GPU command encoding → present
→ publish accessibility tree if the semantics changed
```

Each phase is independently instrumentable and testable; frame budget is
tracked per phase (`Core.FrameStats`, `GOPHICS_PACING`).

## 4. Key API decisions

The decisions that make this a Go library rather than a transliteration.

### 4.1 Widgets are struct values

Struct literals with field names give the ergonomics that other languages get
from named and default parameters:

```go
widget.Column(
    widget.Text{S: "Hello, gophics", Size: th.Type.Heading, Color: th.Text},
    widget.Padding{
        All: 16,
        Child: theme.Button{
            Label: fmt.Sprintf("count: %d", s.n),
            OnTap: s.increment,
        },
    },
)
```

Widgets are small immutable values implementing a `Widget` interface. Go has
no default parameters, so **the zero value is the default** — a discipline
that pervades every widget design in the library.

### 4.2 Interfaces and embedding, not a class hierarchy

Small interfaces with capability checks by type assertion, and shared behavior
through embedded structs (`layout.Base` carries the size/constraints/skip-cache
plumbing). There is no protected-method overriding: where an inheritance-based
design would override `performLayout`, gophics declares an interface method the
embedding struct must provide.

### 4.3 State via generics

```go
type Counter struct{ Start int }

func (c Counter) CreateState() widget.State { return &counterState{} }

type counterState struct {
    widget.StateBase[Counter]   // provides W() Counter and SetState()
    n int
}

func (s *counterState) Build(ctx widget.Ctx) widget.Widget { ... }
```

`StateBase[W]` gives typed access to the current widget configuration without
casts. The build context arrives as the `Build` argument rather than living on
the state.

### 4.4 Keys, context, inherited data

Keys are an interface plus a few concrete types. Inherited data is a
generics-flavored `widget.Provide[T]` / `widget.Of[T](ctx)` pair — the same
O(1) ancestor lookup other frameworks reach for, with better call-site types
than a runtime type-keyed lookup. `MustOf[T]` is the assert-present variant
for values the app guarantees, like a theme or a navigator.

### 4.5 Layout protocol

Constraints down, sizes up, parent sets position; a single layout pass with
relayout-boundary caching. This protocol is language-independent and gophics
keeps it intact, including constraint semantics and hit-test geometry.
Viewport and lazy-list protocols for scrolling build on the same base.

### 4.6 Concurrency: goroutines, one UI owner

**The framework owns one goroutine (the UI loop); all tree mutation happens
there.** `Post(func())` schedules work onto it and any goroutine may call it.
Long work runs in ordinary goroutines and posts results back. The race
detector plus a debug-mode owner check on tree APIs keeps it honest.

This rule also shapes the capability layer: every platform callback is
marshalled through `Post` by a generated wrapper, so an app never observes a
callback on the wrong goroutine regardless of which thread the OS used.

### 4.7 Error handling

Build, layout and paint APIs do not return errors — a widget that fails
renders an error box, collected through a per-tree error handler, and panics
in user callbacks are recovered at phase boundaries so one bad subtree cannot
take down the app. I/O-shaped APIs (asset and image loading) return `error`
normally and have async widget wrappers.

## 5. Rendering: the vector layer

The substrate below the renderer — device, swapchain, shaders, windowing,
WASM, software rasterizer — is vendored and working. What gophics builds on
top is a **2D vector renderer** with UI-grade quality:

- analytic anti-aliasing (tessellate-and-MSAA is not good enough for
  text-adjacent UI edges; hairline strokes and rounded rects must be clean)
- even-odd/nonzero path fills, strokes with joins/caps/miters
- clip stack: rect, rounded-rect and arbitrary path clips
- linear/radial/sweep gradients, image fills with sampling control
- blurs (backdrop and drop shadow) — every design language leans on them
- glyph rendering through an alpha-texture atlas
- layer compositing with opacity, transforms, and offscreen targets

**Two backends behind one `paint.Canvas` interface.**

1. **CPU rasterizer** — analytic-AA scanline filling with clips and
   gradients, blitting tiles to a GPU texture. Correct, testable, and the
   reference the GPU path is diffed against. It is also the floor: it works
   anywhere, including headless CI and hosts with no WebGPU.
2. **GPU vector backend** — still ahead. The target is a **sparse-strips
   hybrid** design (CPU geometry and coverage in SIMD-friendly strips, plus
   lightweight GPU compositing) rather than a pure-compute pipeline. The
   field has spoken on this: Gio shipped a compute renderer derived from
   piet-gpu and **retired it in Jan 2025 as a failed experiment**, and
   Linebender is itself betting on sparse strips over the compute pipeline.
   Sparse strips is also the most Go-portable design — far less compute-shader
   surface to trust — and it degrades gracefully to the CPU backend, which
   shares the strip generator. No Go port exists; this is novel work, taken on
   after the framework exists so it has a full golden-test corpus and a
   reference backend to diff against.

The display-list format (`scene`) serves both: paint records commands,
backends replay them. Damage tracking rides on the layer tree, so a localized
change re-rasters and re-uploads only what moved.

### 5.1 Ecosystem facts & prior art

Load-bearing dependencies, with fallbacks:

| Dependency | Status | Fallback if it stalls |
| --- | --- | --- |
| **wgpu / windowing substrate** (pure-Go WebGPU, Vulkan/Metal via FFI) | vendored; proven incl. WASM + software adapter | a CGo wgpu-native binding — works, but breaks principle #1 |
| **go-text/typesetting** | v0.3.x, active, shared Fyne/Gio governance | none realistic — this is *the* Go text stack; pin and vendor |
| **purego / goffi** | active; Tier 1 incl. iOS/Android | per-platform CGo shims (principle #1 casualty) |
| **image codecs** (wazero/purego webp/avif) | active, CGo-free | stdlib + x/image only |

Prior art worth learning from:

- **The gg rendering stack** — an analytic-AA CPU rasterizer plus a tiered
  GPU pipeline, adopted as a backend behind `paint.Canvas`. Its *rendering*
  holds up; its *text shaping* did not survive scrutiny. A spike
  (`paint/shaping_spike_test.go`, 2026-07-24) found its from-scratch shaper
  applies **zero positional substitution to Arabic** — identical glyph IDs to
  naive per-rune rendering — so its GSUB/GPOS coverage is Latin
  ligatures and kerning only. Two structural cautions came with it:
  measurement bypasses the shaper that drawing uses (a latent layout/paint
  mismatch), and full-surface copies per `Image()` call cost on the web
  present path. Verdict: rendering backend yes, text no — all shaping goes
  through go-text/typesetting, which feeds positioned glyphs down. A later
  GPU-accelerator spike (2026-07-25) found its GPU registration is
  process-global with no CPU readback, so offscreen rendering came back
  blank and broke golden tests; it is gated behind a build tag as
  experimental. The hedge stands: it must remain *a* backend behind the
  Canvas interface, never a foundation.
- **Cogent Core** — the nearest Go toolkit in scope (retained widgets,
  Material 3, go-text stack). Its 2D UI is CPU-rasterized with GPU reserved
  for 3D; gophics's bet on a GPU vector path and a reconciling pipeline is
  the difference. Small bus factor, pre-1.0 — study it, don't build on it.
- **Gio** — a decade of hard-won renderer and platform lessons, dual
  MIT/Unlicense. Its compute-renderer retirement (§5) is a cautionary input.
- **Ebitengine** — the existence proof for CGo-free macOS/Windows and for
  shipping Go on mobile and web at scale. Its windowing lives under
  `internal/`, so gophics copies patterns, not packages.

## 6. The hard problems — risk register

Ordered by how likely each is to stall the project.

### 6.1 Text

Static text turned out better than feared. go-text/typesetting (BSD-3,
jointly governed by Fyne and Gio maintainers, in production in three
toolkits) covers shaping, bidi, segmentation, variable fonts, and — via
`fontscan` — system font discovery with CJK and emoji fallback. Gophics
builds paragraph layout above it.

Two pieces gophics owns and has now built:

- **Bidirectional text.** Paragraph base direction is resolved by the
  Unicode algorithm's first-strong-character rule, isolate-aware, and threaded
  into the segmenter rather than assumed left-to-right. Visual reordering
  implements rule L2 over both embedding levels, so an English phrase inside
  an Arabic sentence keeps its own order while the sentence around it runs
  right to left. Full multi-level UBA remains future work.
- **RTL layout mirroring.** A `Directionality` provider flips rows, and
  directional padding and alignment follow the reading direction. Mirroring
  is opt-in per property rather than redefining left and right, so code that
  means a physical edge — a scrubber, a chart axis — keeps meaning it.

Text **editing** remains the trap: composing regions, IME integration per
platform, selection and caret affinity across bidi boundaries, per-platform
shortcuts. The deliberately bounded position: IME composition works
(preedit is spliced at the caret and underlined until commit), the field is
single-line, and caret geometry is still LTR. The limits are documented at
the type rather than discovered by users.

### 6.2 Renderer quality and performance

Risk: the CPU rasterizer stalls on complexity (large blurs, many layers), or
the GPU path stalls on substrate maturity. Mitigation: a continuous benchmark
suite (frame time per phase on reference scenes) and a two-backend design, so
neither is a single point of failure.

### 6.3 Developer experience without hot reload

Go cannot replicate dynamic-VM hot reload. What gophics does instead, in
order of leverage:

1. **Fast rebuild plus state snapshot.** Go builds are seconds; the dev
   harness restarts the process and restores serializable widget state
   through an opt-in `Snapshottable` interface — hot restart that remembers
   where you were.
2. **Headless preview.** Render a widget subtree with no display and
   re-render on save; the offscreen path makes this nearly free.
3. **Inspector.** Dump the live tree to a browser view.

Live code swap is explicitly out: interpreter-based and plugin-based
approaches are known tarpits (no unload, exact toolchain match, incomplete
module support). The one idea worth a future spike is WASM module swapping
for changed widget code — plausible, shipped by nobody. This remains a real
DX gap and the plan says so.

### 6.4 Mobile

**Status 2026-08-03: GPU present verified on real hardware** (Pixel 10 Pro,
Android 17, Tensor G5 / PowerVR D-Series): ~4–5 ms/frame at 1080x2238@2.625x,
against ~117 ms for the CPU blit path the app had silently been falling back
to. Three bugs blocked it, all fixed: the mobile binaries linked **zero** GPU
backends (wgpu registers backends by import, and the desktop shell got them
via a package mobile never imported — so no device could ever have used the
GPU), a hardcoded surface format the hardware does not offer, and 4 KB ELF
alignment tripping Android's 16 KB page-size dialog. See
`design/mobile-gpu-bringup.md`.

The lesson generalizes past mobile: "GPU unavailable" from wgpu means *no
backend registered*, not *no hardware*. That was misread as an
emulator limitation for weeks on both iOS and Android.

Earlier CPU-path pacing (2026-07-26, M1 Ultra / Metal, 24-row text list at
480x800@2x) is retained because it still describes the CPU fallback: a
localized one-row change costs ~2.6 ms — the common case is smooth — while
full-scene animation every frame costs ~60 ms, because a full-surface raster
*and* a ~6 MB texture upload stack. The present path uploads the whole
surface even when damage is tiny; damage-rect upload is the clearest
remaining lever on that path.

Two rendering bugs remain, both in the backend's GPU tiers rather than the
shell, and both Vulkan-only (the Metal reference render is clean): **GPU text
draws as solid blocks** (glyph positions correct, coverage wrong — pointing at
the glyph-mask atlas format or sampler) and **rotated sprites vanish** on the
direct-surface path. The first blocks any text-bearing Android UI.

Mobile embedding follows the one pattern the Go ecosystem has converged on —
**Go as a library (`gomobile bind`-style .aar/.xcframework) inside a thin
native shell project**. Whole-app mode is bitrotted and every framework ships
its own packaging tool on the bind pattern. Mobile is also the one place CGo
sneaks back in: the FFI layer is Tier-1 on iOS and Android, but the bind
machinery itself uses CGo, so it is contained in `shell/`.

### 6.5 Accessibility

Because gophics draws every pixel, nothing is accessible by default: the OS
sees one opaque image. Bolting a11y on late is how toolkits acquire permanent
accessibility debt, and the Go ecosystem is the evidence.

The design that emerged has three parts:

- **Semantics tree** — collected from the laid-out box tree, cheap, and
  assertable headless. Correctness is testable long before any OS integration
  exists, and it is.
- **Two publishing shapes for one node type.** A flat, ID-addressed
  `A11yNode` serves both a *push* interface (`Accessibility.SetTree`, for
  platforms that want to be handed a tree — the web, where the mirror is a
  DOM) and a *pull* interface (`A11yProvider`, for native APIs that call into
  the app on their own schedule). Same values, no second tree format.
- **Republish on change only.** A screen reader is a second renderer with its
  own budget; the flattened tree is diffed so an animation that changes no
  semantics does not churn the platform's node cache.

Platform state:

- **Android** — complete and verified on device. The bridge exposes the flat
  tree over `gomobile bind`, and the host implements
  `AccessibilityNodeProvider` as a virtual view hierarchy. TalkBack sees each
  row as a clickable node with the right content description and bounds, and
  `ACTION_CLICK` fires the widget's activation.
- **iOS** — complete, simulator-verified. The host view exposes
  `accessibilityElements` as `UIAccessibilityElement`s built from the same
  accessors, with `accessibilityActivate()` wired through. On-device
  VoiceOver inspection is the one validation step still outstanding.
- **Web** — complete. An explorable ARIA DOM mirror over the canvas, one
  element per node, with the canvas marked `aria-hidden`, plus an aria-live
  region for announcements. The mirror is transparent to pointer input, so
  ordinary mouse and touch still reach gophics's own hit testing.
- **macOS** — the tree is published as `NSAccessibilityElement`s on the
  content view, so VoiceOver can explore and activate the UI. Two design
  notes: the list is published flat (the documented shape for custom-drawn
  content, and it spares the user the layout boxes around every field), and
  elements subclass AppKit's own element class rather than implementing the
  accessibility protocol by hand, because the Go callback trampoline cannot
  return the `NSRect` that protocol asks for. Announcements are not wired
  yet — AppKit routes live-region speech through a C function rather than an
  Objective-C method.
- **Linux and Windows** — not implemented. `ctx.Accessibility()` returns nil
  there rather than a sink that silently discards, so an app can tell the
  difference.

An earlier plan routed all of this through a binding to a cross-platform
accessibility C library. That was dropped: the per-platform bridges turned
out to be tractable directly, and one fewer non-Go dependency is worth more
than the shared abstraction.

### 6.6 Go-specific risks

- **GC pauses.** Go 1.26's Green Tea GC keeps stop-the-world pauses well
  under 1 ms — fine for 120 Hz budgets. The real risk is mutator assists
  stealing frame time under high allocation rates, so the rule is allocation
  discipline, not pause fear: zero allocations in layout and paint of
  unchanged subtrees, pooled display lists and geometry, benchmark-gated.
- **WASM.** No threads, larger binaries, GC and `syscall/js` overhead. Web
  stays a first-class CI target so regressions surface immediately.
- **Ecosystem bus factor.** The GPU substrate and the text stack are
  load-bearing external work. Mitigation: pin versions, vendor, contribute
  fixes upstream, and keep the software rasterizer as a floor.

## 7. State of the work

The original M0–M9 sequencing is complete; what follows is what that
produced and what remains. The ordering *was* the estimate, and it held:
every phase de-risked the ones after it.

### Done

- **Foundations** — `geom`, the shell interface, and desktop + web frame
  loops from one `main`.
- **Canvas and CPU renderer** — `paint`/`scene`, display lists, layer tree,
  damage tracking, golden-test harness, per-phase benchmarks.
- **Layout** — render-object protocol, box model, flex, stack, grid, wrap,
  padding/align/constrained, relayout boundaries, hit-test geometry,
  viewport culling by ink bounds.
- **Widgets** — reconciler, State, keys, `Provide`/`Of`, error boundaries,
  semantics, overlays, navigation, lazy lists, selection, rich text.
- **Interaction and motion** — gesture routing with axis and priority
  disambiguation, tickers, curves, controllers, implicit animations, focus,
  keyboard, drag and drop.
- **Scrolling** — lazy lists with measured-height caching, fling physics,
  overscroll, keyboard-avoiding insets.
- **Text editing and theme** — IME-correct single-line input, and the
  default design language: buttons, checkboxes, switches, sliders, radios,
  segmented controls, dropdowns, tabs, tables, dialogs, menus, sheets,
  snackbars, date and time pickers, progress, spinners, tooltips, list
  tiles, chips, badges, avatars.
- **Internationalization** — locale-aware number, money and date formatting;
  bidirectional text; RTL layout mirroring.
- **Accessibility** — semantics tree plus Android, iOS, web and macOS
  bridges (§6.5).
- **Platform capabilities** — around twenty optional capabilities behind
  `ctx.<Cap>()`, including file pickers with native panels on macOS, the
  standard desktop chooser chain on Linux and BSD, and the common dialogs on
  Windows.
- **Mobile** — Android and iOS via the bind pattern, with GPU present
  verified on real Android hardware (§6.4).
- **Terminal** — a fourth shell target, rendering the same widget trees.
- **DX** — headless rendering, golden tests, state-snapshot hot restart,
  inspector, docs site with live WASM demos.

### Remaining, roughly in order of value

1. **GPU vector backend** (§5) — the sparse-strips renderer. The CPU path is
   the fallback and the reference either way.
2. **Vulkan glyph coverage and rotated sprites** (§6.4) — blocks
   text-bearing Android UI on the GPU path.
3. **Accessibility bridges** for Linux and Windows, plus macOS
   announcements and on-device VoiceOver validation on iOS (§6.5).
4. **Damage-rect texture upload** on the CPU present path (§6.4).
5. **Text editing depth** — multi-line, RTL caret geometry, full UBA.
6. **Message catalog and plural rules** in `intl` — formatting is done,
   translation lookup is not.
7. **Desktop platform gaps** — native menu bars, system tray, multi-window,
   and the battery, gamepad and geolocation capabilities that are still
   stubs.
8. **Widget catalog gaps** — draggable scrollbars, reorderable lists,
   pull-to-refresh, tree views, autocomplete.

## 8. Testing strategy

- **Golden images** everywhere, rendered headless through the software path,
  with per-backend tolerance for GPU.
- **Conformance corpora.** Layout and widget test suites from mature
  toolkits encode a decade of edge cases and translate almost mechanically;
  porting them alongside each area is close to free coverage. Licenses are
  respected and attribution is explicit.
- **Fuzzing** — the reconciler (random mutations against a naive rebuild
  oracle), layout (random constraints, with the invariant that a size always
  satisfies its constraints), text (random Unicode through paragraph layout).
- **Benchmarks as tests** — frame-phase budgets and steady-state allocation
  counts asserted in CI, not merely graphed.
- **Race detector** on the full suite, plus debug-mode UI-goroutine ownership
  assertions.
- **Semantics assertions** — accessibility correctness is checked on the
  tree, headless, independent of whether a platform bridge exists.

## 9. Repo layout

```
gophics/
  geom/          # Point, Size, Rect, Insets, transforms
  input/         # device-independent input types
  paint/         # Canvas, Path, Paint, Color, gradients, images
  text/          # shaping, bidi, paragraph layout, fonts
  scene/         # display lists, layer tree, damage tracking
  layout/        # render-object protocol, box/flex/stack/viewport, semantics
  widget/        # Widget/Element, State, keys, focus, reconciler, gestures
  anim/          # tickers, curves, controllers
  theme/         # the default design language
  chart/         # data visualization
  intl/          # locale-aware formatting
  sound/         # audio mixer
  shell/         # interface + desktop/ web/ mobile/ terminal/
  app/           # the runner tying shell → scene → widget; Headless
  cmd/gophics/   # dev CLI: build, run and hot-restart across every target
  internal/gfx/  # vendored GPU substrate (wgpu, naga, windowing)
  examples/      # ~25 apps, from hello to a beancount ledger
  design/        # ADRs and design notes
  docs/          # the site, live WASM demos, reviews
  skills/        # agent instructions + API-drift checker for this repo
```

`build/` and `gallery/` hold build output and are untracked.

## 10. Working method

- Decisions that shape the API become short ADRs in `design/adr/` when
  settled. Sketches in this document are reasoning, not commitments.
- When in doubt about *behavior* — a layout edge case, a gesture
  disambiguation rule — match a well-tested implementation and cite it in a
  comment. When in doubt about *API*, design for Go.
- Dependencies are replaceable, not relationships. Pin and vendor the
  load-bearing ones; send a fix upstream when it's trivial. If a dependency
  stalls or drifts, fork it privately or regenerate the needed subset behind
  the owning interface. The exception by necessity is the text stack, whose
  shaping correctness is not regenerable on demand: it gets pinned, vendored,
  and watched.
- Document limits at the type, not in an issue tracker. A `TextField` that
  says it is single-line and LTR-caret is more useful than one that leaves
  the user to find out.
