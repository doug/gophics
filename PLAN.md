# Gophics — a cross-platform UI library for Go

Gophics is a UI library for Go: it draws every pixel itself on a pure-Go GPU
stack with zero CGo, so one codebase renders the same interface on desktop,
web (WASM), iOS, Android, and a terminal.

This document is the architecture rationale — the *why* behind the pipeline
and the decisions that shaped the API. It began as a plan and is kept as
reasoning. **Status: built.** The pipeline described here works on all listed
targets; §7 tracks what is done and what is not. Read this for design
reasoning and the git history for the current state.

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
6. **No codegen in your build.** `go build` compiles what you wrote and a
   stack trace names it — no generated layer between you and your program;
   structs, generics, stdlib. The framework generates its own plumbing
   (`internal/capgen`, §7's tables), which is on this side of the line and
   which an app never runs. For a new generator the test is not "is it
   codegen" but *does this remove duplication or merely automate it* — a stub
   from a schema you do not own is the first; forty per-app functions because
   a package was factored wrong is the second, and the fix is the factoring.
7. **Small surface, Go-style stability.** A distilled API and the Go 1
   compatibility ethos after 1.0 — no upgrade treadmill.

The corollary is a center of gravity: desktop tools, UIs embedded inside
larger Go programs, server-side rendering, single-binary distribution. Mobile
is supported and was sequenced late (§6.4) because that ordering de-risked
everything before it.

## 2. Scope inventory

What a toolkit of this kind needs, and how gophics gets it. "Build" means
gophics owns the code; "adopt" means a dependency does the work.

This table records a decision, not a status. It used to carry a State column
as well, and that column drifted — it still called accessibility on Linux and
Windows pending after both shipped, and text editing single-line after
multiline landed. Where each subsystem stands is §7, which is generated.

| Subsystem | What it is | Approach |
| --- | --- | --- |
| GPU rasterization, compositing | vector renderer on wgpu | **Build** (§5) |
| Text shaping, bidi, line breaking, font discovery | HarfBuzz-class stack | **Adopt** go-text/typesetting + `fontscan` |
| Image decoding | png/jpeg/webp/… | **Adopt** stdlib + x/image |
| Platform embedding | windows, input, vsync, IME | **Adopt/extend** the windowing layer |
| Canvas, Paint, Path, Scene | immediate + retained drawing | **Build** — `paint`, `scene` |
| Render objects, box model, flex, stack, viewport | layout protocol | **Build** — `layout` |
| Widget/element trees, State, reconciliation, focus, routing | the declarative layer | **Build** — `widget` |
| Pointer routing, gesture disambiguation | gestures | **Build** — in `app`/`widget` |
| Tickers, curves, controllers | animation | **Build** — `anim` |
| Frame pipeline, vsync phases | scheduling | **Build** — `app` |
| Default design language | theme + controls | **Build** — `theme` |
| Semantics + platform a11y bridges | accessibility | **Build** (§6.5) |
| Text editing + IME | TextField, composing regions | **Build** (§6.1) |
| Internationalization | number/date/money formats, bidi, RTL layout | **Build** — `intl` + `widget` |
| Inspector, preview, dev harness | DX | **Build** (§6.3) |

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
2. **GPU vector backend** — partly here, and the entry below understated it.
   A traditional GPU vector renderer already runs: stencil path fills, convex
   fills, SDF shapes, glyph-mask and MSDF text, textured quads, blending,
   compositing, depth clip and backdrop blur are live pipelines, diffed against
   the CPU reference by the `gophics_gpu` tests. What is still ahead is the
   **sparse-strips** upgrade (CPU geometry and coverage in SIMD-friendly
   strips, plus lightweight GPU compositing) rather than a pure-compute
   pipeline. `strip.wgsl` and the tilecompute stage shaders are written; the
   pipelines are stubs (`StubComputePipelineID`, "when wgpu is ready"), and
   tilecompute is currently used only for CPU-side curve flattening. The
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

Two rendering bugs were tracked here — **GPU text drawing as solid blocks**
and **rotated sprites vanishing** on the direct-surface path, both Vulkan-only.
Both are fixed, retested on a Pixel 10 Pro (2026-08-16, Vulkan, direct
surface):

- Text renders crisply at every size, from a 32 px balance figure down to
  10 px chart axis labels. The likely fix is the surface-format work, which
  was the same class of fault: an attachment format that did not match what
  the pipeline was compiled for.
- The `gophics_verify` bring-up scene draws its plain / tinted / **rotated**
  sprite trio in full, along with gradients, path fill, nested opacity and
  backdrop blur. Rotated sprites were fixed by routing the non-axis-aligned
  case to the textured-quad path (`gg/gpu`, 2a5f24b) rather than dropping to a
  CPU fallback the direct-surface path discards.

Retest with `gophics run -p android -tags gophics_verify ./examples/hn/mobile`;
the desktop reference to diff against is
`GPUCHECK_SHOT=out.png go test -run TestGPUCheckRenders ./examples/gpucheck`.

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

The original M0–M9 sequencing is complete; the ordering *was* the estimate, and
it held — every phase de-risked the ones after it.

What follows is split deliberately. The tables are generated from the tree by
`scripts/tools/planfacts.py` and checked by `scripts/gates.sh`, because this
section used to be maintained by hand and went stale six times in a single
week: native menus and mobile lifecycle sat under "remaining" after they
shipped, tree views were listed as missing while `widget/tree.go` was in the
repo, and the repo layout named a package that no longer existed. An inventory
kept by hand next to the thing it inventories is a promise to notice, and
nobody does.

The prose keeps what no generator can produce: which of the gaps is worth
closing next, and why.

### What ships, by platform

<!-- planfacts:capabilities -->
| Capability | desktop | web | mobile | terminal |
|---|---|---|---|---|
| `Accessibility` | yes | yes | — | — |
| `Audio` | yes | yes | yes | yes |
| `Battery` | yes | yes | — | — |
| `Camera` | yes | yes | yes | — |
| `CameraPreview` | yes | yes | yes | — |
| `Connectivity` | yes | yes | — | — |
| `FilePicker` | yes | yes | — | — |
| `Gamepads` | yes | yes | hollow | — |
| `Geolocation` | — | yes | — | — |
| `Haptic` | — | yes | yes | — |
| `Lifecycle` | yes | yes | yes | — |
| `Links` | yes | yes | — | — |
| `Menus` | yes | — | — | — |
| `Microphone` | yes | yes | yes | yes |
| `Notifier` | — | yes | — | — |
| `Permissions` | — | yes | — | — |
| `Preferences` | yes | yes | — | — |
| `SecureStorage` | — | yes | — | — |
| `Share` | — | yes | — | — |
| `Socket` | yes | yes | yes | — |
| `TextInput` | — | yes | — | — |
| `Tray` | yes | — | — | — |
| `WebView` | — | yes | — | — |
| `WindowControl` | yes | yes | — | — |

*Generated. `yes` means the shell publishes the accessor; `—` means `ctx.<Cap>()` is nil there, which is how an app is meant to ask. `hollow` means it returns a value whose methods do nothing — the one shape a caller cannot detect.*
<!-- /planfacts -->

Terminal publishes no optional capability, which is a fact about terminals
rather than a gap. Web is the most complete shell — that is not a standard the
others fall short of, it is the platform whose APIs happen to match this
capability set.

### The widget catalog

<!-- planfacts:widgets -->
`Align`, `AspectRatio`, `Autocomplete`, `Canvas`, `Decorated`
`Directionality`, `Dismissible`, `DragHost`, `Draggable`, `DropTarget`
`Fill`, `Flexible`, `Grid`, `Hero`, `Image`, `Interactive`
`KeyboardAvoiding`, `LayoutBuilder`, `LazyList`, `Nav`, `Navigator`
`NetworkImage`, `Opacity`, `Overlay`, `OverlayHost`, `Padding`
`Reorderable`, `Rich`, `SafeArea`, `Scroll`, `ScrollController`
`SelectableText`, `SelectionArea`, `Semantics`, `Sized`, `Stack`, `Text`
`TextField`, `Transform`, `Tree`, `TreeNode`, `Wrap`

*Generated: every exported widget type.*
<!-- /planfacts -->

### Where to go next

Ordered by value, and this ordering is a judgment rather than a reading of the
tree.

1. **Honesty before coverage.** Three capabilities answer without meaning
   anything, which is worse than being absent because a caller cannot detect
   it: mobile `Gamepads` returns a poller that never reports a pad, Windows
   `AnnounceA11y` has an empty body, and `ctx.Accessibility()` is nil on mobile
   even though mobile publishes a full semantics tree through the Bridge — so a
   mobile app cannot announce to a screen reader at all. Each is small, and
   each currently misleads.
2. **GPU vector backend** (§5) — the sparse-strips renderer. The CPU path is
   the fallback and the reference either way.
3. **Mobile's thin shell.** `Permissions` is nil on the one platform with a
   real permission model; `SecureStorage` is nil, so an app has nowhere safe
   for a token; `Share` is nil on both platforms that have a share sheet.
4. **Damage-rect texture upload** on the CPU present path (§6.4) — damage is
   tracked and the raster is already damage-culled (`ReplayDamaged`); only the
   upload sends the whole surface. It is *not* the one-line change it looks
   like, and the trap is worth stating because the API invites it:

   `Painter.PresentGPU` already calls `FlushGPUWithViewDamage`, passing an
   empty rect. Filling that rect in would work — the CPU present path is
   blit-only, so damage is honoured rather than dropped with the MSAA warning —
   and would be **wrong**. The two-frame union that makes partial damage safe
   against a recycled swapchain image lives in `ggcanvas.Canvas`
   (`prevFrameDamageRects`, the Wayland buffer-age pattern), and `PresentGPU`
   goes to the context directly, bypassing it. Single-frame damage into a
   double- or triple-buffered surface leaves pixels from two frames ago:
   intermittent, invisible to tests, visible to users.

   So the work is the buffer-age accounting on this path, not the plumbing.
5. **Accessibility, the remainder** (§6.5) — Windows `BoundingRectangle`
   arrives at clients as zero, and `Expanded` is dropped at the desktop bridge
   (web already emits `aria-expanded`). iOS has only been checked in the
   simulator.
6. **Desktop's missing pieces** — no `Notifier` on any of the three operating
   systems, no `SecureStorage`, no `Share`. `WebView` is a large piece of work
   on every platform and is the honest last of these.
7. **Text editing depth** — multi-line landed (`TextField.Multiline`); RTL
   caret geometry and full UBA have not.
8. **Durable background work** on mobile — see M7. Deliberately last: it is the
   largest, and the design deliberately makes it useful before any platform
   scheduler exists.

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

<!-- planfacts:layout -->
| Package | Purpose |
|---|---|
| `anim/` | Animation primitives: curves and controllers |
| `app/` | Ties the widget tree to a shell: the gophics runtime |
| `apptest/` | Drives a gophics UI in a test and asserts on what it produced |
| `chart/` | A built-in, Swift Charts–style charting library |
| `cmd/` | Developer CLI: build, run and hot-restart across every target |
| `examples/` | Example apps, from hello to a beancount ledger |
| `geom/` | The geometric primitives used throughout gophics |
| `input/` | Per-frame, poll-style input state for games: which keys are held right now |
| `internal/` | Vendored GPU substrate (wgpu, naga, windowing) and internals |
| `intl/` | Formats numbers, money and dates the way a reader's locale writes them |
| `layout/` | Gophics's render layer: the box protocol and the core layout boxes |
| `paint/` | Gophics's drawing layer |
| `scene/` | Display lists: recorded paint commands that can be replayed onto any paint.Canvas |
| `shell/` | Defines the platform interface gophics runs on |
| `sound/` | A pure-Go DSP mixer for game and UI audio: PCM samples, oscillators, gain |
| `text/` | Gophics's text stack: shaping, bidi, font fallback, and line breaking |
| `theme/` | Gophics's default design language: a Theme value provided to the tree plus components |
| `widget/` | Gophics's declarative layer: immutable widget values describing the UI |

*Generated from each package's doc comment.*
<!-- /planfacts -->

`build/` holds build output and is untracked. `design/` holds ADRs and design
notes, `docs/` the site and its live WASM demos, `skills/` the agent
instructions and API-drift checker for this repo.

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
