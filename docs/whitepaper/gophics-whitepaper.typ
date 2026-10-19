// Gophics — executive technical white paper.
// Build: typst compile gophics-whitepaper.typ
//
// Fonts are named explicitly and verified present via `typst fonts`; a silent
// fallback would change the document's voice without failing the build.

#let ink      = rgb("#14181C")
#let body     = rgb("#22292F")
#let muted    = rgb("#5C6773")
#let accent   = rgb("#0B6E7F")
#let rule     = rgb("#D4DCDF")
#let wash     = rgb("#F4F7F8")

// Severity is encoded as a word as well as a colour: a reader printing this in
// greyscale, or reading it with a colour-vision deficiency, must still be able
// to rank the findings.
#let blocking = rgb("#9A3324")
#let notable  = rgb("#8A6D1F")
#let minor    = rgb("#4A6741")
#let closed   = rgb("#2F6B53")

#let sans = "Avenir Next"
#let serif = "Charter"
#let mono = "DejaVu Sans Mono"

#set document(
  title: "Gophics — Technical Assessment",
  author: "Doug Fritz",
)

#set page(
  paper: "a4",
  margin: (left: 26mm, right: 22mm, top: 24mm, bottom: 22mm),
  footer: context {
    set text(font: sans, size: 7.5pt, fill: muted)
    grid(
      columns: (1fr, auto),
      align: (left, right),
      [Gophics — Technical Assessment],
      [#counter(page).display("1")],
    )
  },
)

#set text(font: serif, size: 10pt, fill: body, lang: "en")
#set par(justify: true, leading: 0.62em, spacing: 0.9em)

#set heading(numbering: "1.1")

// Sections do NOT force a page break. With several sections running to about a
// page and a half, forcing one stranded a few table rows on an otherwise blank
// page three times over. Generous space above does the separating instead, and
// the block is unbreakable so a heading never sits alone at a page foot.
#show heading.where(level: 1): it => {
  block(above: 30pt, below: 14pt, breakable: false)[
    #set text(font: sans, size: 17pt, weight: 600, fill: ink)
    #block(below: 7pt)[
      #text(font: sans, size: 9pt, fill: accent, weight: 600)[
        #counter(heading).display(it.numbering)
      ]
    ]
    #it.body
    #block(above: 9pt)[#line(length: 100%, stroke: 0.8pt + accent)]
  ]
}

#show heading.where(level: 2): it => block(above: 16pt, below: 7pt)[
  #set text(font: sans, size: 11.5pt, weight: 600, fill: ink)
  #counter(heading).display(it.numbering) #h(0.6em) #it.body
]

#show heading.where(level: 3): it => block(above: 12pt, below: 5pt)[
  #set text(font: sans, size: 9.5pt, weight: 600, fill: accent)
  #upper(it.body)
]

#show raw: set text(font: mono, size: 8.5pt)
#show link: set text(fill: accent)
#show strong: set text(fill: ink, weight: 600)

// A labelled aside. Used for verdicts and judgement calls, so a reader
// skimming only these gets the argument.
#let verdict(title, content) = block(
  width: 100%,
  fill: wash,
  stroke: (left: 2.5pt + accent),
  inset: (left: 12pt, rest: 11pt),
  above: 14pt, below: 14pt,
)[
  #set text(size: 9.5pt)
  #text(font: sans, size: 8.5pt, weight: 600, fill: accent)[#upper(title)]
  #block(above: 6pt)[#content]
]

#let chip(c, label) = box(
  fill: c,
  inset: (x: 5pt, y: 2.5pt),
  radius: 1.5pt,
  text(font: sans, size: 7pt, weight: 600, fill: white)[#upper(label)],
)

#let th(s) = table.cell(
  align: bottom,
  text(font: sans, size: 8pt, weight: 600, fill: ink)[#s],
)

#let datatable(cols, ..cells) = table(
  columns: cols,
  stroke: none,
  inset: (x: 0pt, y: 6pt),
  column-gutter: 14pt,
  align: left + top,
  ..cells,
)

// A numbered finding. The numeral is its own top-aligned column so it cannot
// ride into the text beside it, which is what a bottom-aligned table cell did.
#let finding(n, content) = grid(
  columns: (14pt, 1fr),
  column-gutter: 10pt,
  align: (right + top, left + top),
  text(font: sans, size: 11pt, weight: 600, fill: accent)[#n],
  content,
)

// ─────────────────────────────────────────────────────────── cover

#block(above: 18mm)[
  #text(font: sans, size: 8.5pt, weight: 600, fill: accent)[
    #upper("Technical white paper")
  ]
]

#block(above: 10pt, below: 0pt)[
  #set text(font: sans, size: 44pt, weight: 600, fill: ink)
  #set par(leading: 0.3em)
  Gophics
]

#block(above: 12pt)[
  #set text(font: serif, size: 15pt, fill: muted, style: "italic")
  #set par(leading: 0.45em)
  Cross-platform native UI in pure Go — what it does, \
  what it is missing, and what it would take to compete.
]

#block(above: 16pt)[#line(length: 42%, stroke: 1.2pt + accent)]

#block(above: 14pt)[
  #set text(font: sans, size: 8.5pt, fill: muted)
  #datatable(
    (auto, 1fr),
    [Subject], [`github.com/doug/gophics` — assessed at `v0.1.0`, commit `dbafdb5`],
    [Date], [15 October 2026],
    [Scope], [Architecture, implementation state, competitive gap analysis against
              Jetpack Compose and Flutter],
    [Basis], [Direct inspection of the source tree; see Appendix A for method and limits],
  )
]

#v(14pt)

#verdict("Verdict")[
  Gophics is *architecturally complete and strategically under-positioned.* The
  core is not a prototype: five platform backends, accessibility on all of them,
  measured per-platform scroll physics, a real text stack, 11,753 tests and a
  zero-CGo invariant enforced in CI. The gaps that remain are mostly *not* in the
  framework — they are a short list of specific rendering and text omissions, and
  a long list of things that are not code at all: an ecosystem, a stability
  promise, packaging, and documentation.

  Feature-matching Flutter is a decade of work and the wrong goal. The defensible
  position is the one no other toolkit occupies: pure Go, own-rendering,
  single-binary, headless-testable — which is decisive for internal tools,
  developer tooling, embedded and kiosk displays, and Go-native organisations,
  none of which the incumbents serve well.
]

// One deliberate break: the cover stands alone. Sections do not break, but a
// title page that runs straight into body text is not a title page.
#pagebreak()

// ─────────────────────────────────────────────────────────── 1

= Executive summary

Gophics reimplements the rendering architecture behind Flutter and Jetpack
Compose — immutable widgets, element reconciliation, constraint layout, GPU layer
compositing — in idiomatic Go, on a vendored pure-Go WebGPU stack. It draws every
pixel itself, so one codebase renders the same interface on macOS, Windows,
Linux, the web, iOS, Android, and a terminal, with no platform forks and no
embedded webview.

The central engineering constraint is *zero CGo*: `import "C"` anywhere fails
CI. Everything reaches the operating system through syscalls, dynamically loaded
symbols, or JavaScript interop. This is not an aesthetic preference. It is what
makes `go build` the entire toolchain, what lets binaries cross-compile from any
host to any target, and what lets a test binary be pushed to a phone over `adb`
and run there with no application wrapper, no JNI, and no emulator.

#heading(level: 3, outlined: false)[Five findings]

#block(above: 10pt, below: 6pt)[
  #stack(
    spacing: 11pt,

    finding(1)[*The core is more complete than its size suggests.* Accessibility
      is implemented on all five targets — macOS `NSAccessibility`, Windows UI
      Automation, Linux AT-SPI, a web DOM mirror, and Android's
      `AccessibilityNodeProvider`. Flutter's desktop accessibility is materially
      weaker than this.],

    finding(2)[*The engineering discipline is unusually high.* 11,753 test
      functions, 913 benchmarks, golden-image tests with cross-machine
      tolerance, a generated public-API manifest that fails the build on drift,
      and a generated capability matrix that cannot lie about platform support.],

    finding(3)[*The remaining rendering gaps are few but visible.* Colour emoji
      render as nothing. The web target has no system font fallback. The
      painting API has two-stop linear gradients and no blend modes. Each is
      bounded work.],

    finding(4)[*The real deficits are not engineering.* No third-party package
      ecosystem, no API stability commitment, no desktop packaging, and no
      native platform views. Hot reload has left this list: it is impossible in
      Go on every platform, and what the project ships in its place —
      state-preserving hot restart on desktop, web, emulator and simulator —
      is a different and defensible claim.],

    finding(5)[*Payload and binary size are not the problem they are assumed to
      be.* A counter application compiles to 2.73~MB of Brotli-compressed
      WebAssembly and a 20.6~MB desktop binary — the same order as Flutter,
      measured directly rather than estimated.],
  )
]

// ─────────────────────────────────────────────────────────── 2

= What Gophics does

== The proposition

Most cross-platform toolkits force a choice between consistency, native feel,
and toolchain weight. Gophics occupies a corner none of the incumbents do.

#datatable(
  (auto, 1fr),
  th[Approach], th[Trade-off accepted],
  table.hline(stroke: 0.5pt + rule),
  [Webview wrappers \ #text(size: 8pt, fill: muted)[Electron, Tauri, Wails]],
  [Near-single-binary, but the interface is HTML and inherits each platform's
   webview quirks.],

  [Native-widget bridges \ #text(size: 8pt, fill: muted)[React Native]],
  [Real OS widgets, but a serialising bridge and per-platform inconsistencies to
   chase.],

  [Own-rendering SDKs \ #text(size: 8pt, fill: muted)[Flutter, Compose Multiplatform]],
  [Pixel-perfect and consistent, but a whole SDK, an engine artifact, and a new
   language.],

  [Own-rendering, not a toolkit \ #text(size: 8pt, fill: muted)[Gio, Ebitengine]],
  [The right idea in Go, but immediate-mode or game-loop rather than polished
   widgets.],

  table.hline(stroke: 0.5pt + rule),
  [*Gophics*],
  [*Own-rendering and pure Go.* Consistency of an own-rendering toolkit, a single
   static binary, headless-testable with `go test` — without leaving Go.],
)

== Architecture

The pipeline is Flutter's, with Go's idioms rather than Dart's.

#datatable(
  (auto, 1fr),
  th[Stage], th[Responsibility],
  table.hline(stroke: 0.5pt + rule),
  [`widget`], [Immutable widget descriptions; `State` objects; element
    reconciliation keyed on type, sibling position and explicit keys.],
  [`layout`], [Constraint-based layout — constraints down, sizes up — over a
    retained box tree in logical pixels.],
  [`paint`], [A `Canvas` of retained draw commands, diffed frame to frame to
    produce a damage rectangle for partial repaint.],
  [`internal/scene`], [Records and diffs the command stream; computes damage from
    real glyph ink, not advance boxes.],
  [`internal/gfx`], [Vendored pure-Go WebGPU stack. Hardware abstraction layers
    for Metal, Vulkan, OpenGL ES, plus software and null backends.],
  [`shell/*`], [Platform backends: `desktop`, `web`, `mobile`, `terminal`.],
)

A deliberate hazard is documented in the repository's own guidance and worth
repeating, because it is the kind of defect that is invisible during
development: everything in the widget tree is in *logical* pixels, while the
painter's surface, damage rectangles and textures are *physical* — logical
multiplied by device scale. Mixing them is correct at 1× and wrong at 2×, where
a logical rectangle names the top-left quadrant of a physical surface. That
defect has shipped here before.

== Two invariants that shape everything

#heading(level: 3, outlined: false)[Zero CGo]

`import "C"` fails CI. Native integration is reached three ways: an internal FFI
package using `dlopen`/`dlsym` with assembly trampolines on macOS and Linux
(driving the Objective-C runtime directly); `syscall.SyscallN` over COM vtables
on Windows; and on mobile, a bridge where Go enqueues a request that a Kotlin or
Swift host drains — the native code lives in the host, and Go stays pure.

The payoff is concrete. Test binaries cross-compile and run on real devices with
no application wrapper:

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -o app.test.android ./app
adb push app.test.android /data/local/tmp/ && adb shell ./app.test.android
```

Neither Flutter nor Compose Multiplatform can do this. The repository notes that
a second backend's opinion, obtained this way, has repeatedly disagreed with the
development Mac's — which is the point.

#heading(level: 3, outlined: false)[Capabilities are opt-in, and `nil` is a real answer]

Platform services are plain Go interfaces. A shell implements
`shell.BatteryWindow` or it does not, and `ctx.Battery() == nil` means "this
platform cannot tell you," which an application uses to hide an affordance
rather than to show one that never works.

#verdict("A design rule worth borrowing")[
  The project forbids publishing a capability whose implementation always
  returns nothing. A caller cannot distinguish "no controller connected" from
  "this build cannot see controllers," so it would show a pairing prompt
  forever. One capability — desktop connectivity — previously reported
  `online = true` unconditionally; it is now `nil`, which is the honest answer.
  This is a stronger contract than Flutter's plugin model offers.
]

Only the interface and the per-platform implementation are hand-written. The
dispatch layer — the capability struct, the context accessors, the runner wiring,
and thread-marshalling wrappers that guarantee every callback arrives on the UI
goroutine — is generated from the interface definitions.

// ─────────────────────────────────────────────────────────── 3

= Verified state of the implementation

== Scale

#datatable(
  (1fr, auto),
  th[Measure], th[Value],
  table.hline(stroke: 0.5pt + rule),
  [Go packages], [163],
  [First-party framework code (excl. vendored graphics, examples, tests)], [60,533 lines],
  [Vendored graphics substrate (`internal/gfx`, MIT lineage)], [351,325 lines],
  [Test functions / test files], [11,753 / 1,106],
  [Benchmarks], [913],
  [Example applications], [30],
  [Direct module dependencies], [9],
  [Platform backends], [4 (desktop, web, mobile, terminal) + headless],
  [GPU hardware abstraction layers], [Metal, Vulkan, OpenGL ES, software, null],
)

The dependency count deserves emphasis. The entire toolkit — shaping, bidi,
audio decoding, WebGPU, mobile binding — rests on nine direct modules, of which
four are `golang.org/x`. A toolkit that pulls in nine dependencies is a toolkit
an organisation can actually audit.

== Measured artifact sizes

Compression figures below are measured, not estimated, for `examples/counter`.

#datatable(
  (1fr, auto, auto, auto),
  th[Artifact], th[Raw], th[gzip -9], th[Brotli -11],
  table.hline(stroke: 0.5pt + rule),
  [WebAssembly (`js/wasm`)], [15.13 MB], [3.83 MB], [*2.73 MB*],
  [Desktop binary (`darwin/arm64`)], [20.65 MB], [—], [—],
)

The raw WebAssembly figure invites alarm and should not. What a browser
downloads is the compressed artifact, and 2.73~MB is the same order as
Flutter's web engine. This is a competitive parity result, not a liability —
though neither the development server nor the documentation server currently
negotiates compression, so the figure depends on the host doing it.

== Capability coverage

The matrix below is generated from the source tree by the project's own tooling
and the build fails when it drifts. `yes` means the backend publishes the
accessor; #sym.dash.em means `ctx.<Cap>()` is `nil` there, which is how an
application is meant to ask.

#set text(size: 8.5pt)
#table(
  columns: (1fr, auto, auto, auto, auto),
  stroke: none,
  inset: (x: 4pt, y: 3.6pt),
  align: (left, center, center, center, center),
  fill: (_, y) => if calc.odd(y) { wash },
  table.header(th[Capability], th[desktop], th[web], th[mobile], th[terminal]),
  table.hline(stroke: 0.5pt + rule),
  [Accessibility], [yes], [yes], [yes], [—],
  [Battery], [yes], [yes], [yes], [—],
  [Biometric], [—], [—], [yes], [—],
  [Camera], [yes], [yes], [yes], [—],
  [CameraPreview], [yes], [yes], [yes], [—],
  [Connectivity], [—], [yes], [yes], [—],
  [Cursor], [yes], [yes], [—], [—],
  [FilePicker], [yes], [yes], [yes], [—],
  [FolderPicker], [yes], [yes], [—], [—],
  [Gamepads], [yes], [yes], [—], [—],
  [Geolocation], [—], [yes], [yes], [—],
  [Haptic], [—], [yes], [yes], [—],
  [Lifecycle], [yes], [yes], [yes], [—],
  [Links], [yes], [yes], [yes], [—],
  [Locale], [—], [yes], [yes], [—],
  [Menus], [yes], [—], [—], [—],
  [Microphone], [yes], [yes], [yes], [yes],
  [Notifier], [yes], [yes], [yes], [—],
  [Permissions], [—], [yes], [—], [—],
  [Photos], [—], [—], [yes], [—],
  [Preferences], [yes], [yes], [yes], [yes],
  [SecureStorage], [yes], [yes], [yes], [—],
  [Share], [—], [yes], [yes], [—],
  [Socket], [yes], [yes], [yes], [—],
  [Speakers], [yes], [yes], [yes], [yes],
  [TextInput], [—], [yes], [yes], [—],
  [Tray], [yes], [—], [—], [—],
  [WakeLock], [—], [—], [yes], [—],
  [WebView], [—], [yes], [—], [—],
  [WindowControl], [yes], [yes], [—], [—],
)
#set text(size: 10pt)

Three entries are gaps rather than deliberate absences: desktop `FilePicker` is
a genuine `NSOpenPanel` on macOS but unimplemented on Linux and Windows;
`Connectivity` is absent on desktop pending a reachability source; and
`Permissions` exists only on the web.

== Engineering discipline

Several practices here are stronger than the industry norm and materially reduce
the risk of adopting the project.

#datatable(
  (auto, 1fr),
  th[Practice], th[Effect],
  table.hline(stroke: 0.5pt + rule),
  [Generated API manifest], [The public surface is recorded in a checked-in
    file; adding a name requires regenerating it, so every API change appears as
    a reviewable diff.],
  [Generated capability matrix], [Platform-support claims are derived from the
    tree, not maintained by hand. The previous hand-written table was wrong in
    both directions simultaneously.],
  [Golden-image tests], [Rendering is pinned to reference images with an
    anti-aliasing tolerance, because exact comparison fails across machines on
    sub-least-significant-bit rounding.],
  [Measured platform behaviour], [A tracing harness recorded real scroll decay on
    iOS, macOS and Android; the three curves are modelled separately rather than
    approximated by one constant.],
  [On-device test binaries], [Zero CGo means every package's tests can run on a
    phone or a Windows machine without a wrapper.],
  [Build-tag matrix], [A pure-CPU configuration and a GPU configuration are
    compiled and tested separately, so the graphics substrate can be excluded
    entirely.],
)

// ─────────────────────────────────────────────────────────── 4

= Where Gophics is already ahead

It is worth being explicit about what should *not* be rebuilt, because the
natural instinct when facing a gap list is to assume the whole core needs work.

#datatable(
  (auto, 1fr),
  th[Dimension], th[Position],
  table.hline(stroke: 0.5pt + rule),
  [Toolchain weight], [`go build` is the entire story. No SDK, no engine
    artifact, no toolchain doctor. Neither incumbent comes close.],
  [Testability], [Golden images, fuzzing, benchmarks and profiling through
    ordinary `go test`, with no emulator or bespoke harness. Flutter's widget
    tests are weaker and its integration tests need a device.],
  [Desktop accessibility], [Implemented on all three desktop platforms. This is
    a known weak point for Flutter and a legal requirement in enterprise and
    government procurement.],
  [Scroll feel], [Per-platform decay curves derived from device measurement.
    Most own-rendering toolkits ship one constant and feel wrong on at least two
    platforms.],
  [Capability honesty], [`nil` distinguishes "unsupported" from "no data," so
    applications degrade correctly instead of showing dead affordances.],
  [Concurrency], [Goroutines with a single UI thread. Streaming live data into a
    view — awkward in most UI stacks — is the easy case.],
  [Terminal backend], [Neither competitor has one.],
  [Dependency surface], [Nine direct modules, auditable in an afternoon.],
)

// ─────────────────────────────────────────────────────────── 5

= What is missing

== How these are ranked

#block(above: 10pt, below: 12pt)[
  #set text(size: 9pt)
  #grid(
    columns: (auto, 1fr),
    column-gutter: 10pt,
    row-gutter: 7pt,
    chip(blocking, "Blocking"),
    [A developer or an end user meets this in normal use and cannot work around
     it.],
    chip(notable, "Notable"),
    [Reached once an application becomes real; a workaround exists but costs.],
    chip(minor, "Minor"),
    [Narrow or specialised; defensible to defer indefinitely.],
  )
]

== Rendering and text

These are the gaps a developer meets in the first hour, and all are bounded.

#datatable(
  (auto, 1fr, auto),
  th[Gap], th[Detail], th[Rank],
  table.hline(stroke: 0.5pt + rule),

  [Colour emoji], [Glyph outline emission handles vector outlines only and
    returns early for bitmap and SVG glyphs, so a colour emoji draws *nothing*.
    Any application with user-entered text shows invisible characters. The font
    library already decodes the data; the obstacle is that the fast GPU text path
    is an alpha-mask atlas that takes colour as a separate tint, so a multicolour
    glyph cannot pass through it.], chip(blocking, "Blocking"),

  [Web font fallback], [System font fallback is a deliberate no-op on the web
    target: a WebAssembly page has no font directory to scan. Runes outside the
    explicitly registered fonts fall back to the primary face, so CJK, Cyrillic
    and symbols require the application to ship font files it may not know it
    needs. This item was costed as a chore and is not one: a browser cannot hand
    a page the bytes of an installed font at all. The CSS Font Loading API
    exposes no source for a `FontFace`, and `FontFaceSet.check()` returns true
    for families that do not exist, so it cannot even be used to ask. The one
    API that does return bytes is desktop-Chromium-only and permission-gated.
    The gap is therefore *partially closeable only* — by fetching faces on
    demand, never by using the machine's own.], chip(blocking, "Blocking"),

  [Painting expressiveness], [Two-stop linear gradients by axis only — no
    multi-stop, radial or sweep. No blend modes exposed, although the underlying
    context already accepts one on every call. No colour or image filters beyond
    a backdrop blur, round caps and joins only, no dash patterns, no
    arbitrary-path clipping, no custom shaders. Flutter and Compose both offer
    all of this.], chip(notable, "Notable"),

  [Velocity handoff in animation], [Implicit animation is present and well
    designed — a generic animated wrapper with typed helpers for colour, insets,
    scalars, scale and rotation. What is missing is a velocity-aware spring: a
    gesture released mid-flight cannot hand its speed to the animation that
    continues it. A true critically-damped solver already exists but is private
    to the scroll implementation.], chip(notable, "Notable"),
)

#verdict("Closed during this assessment")[
  #chip(closed, "Resolved") #h(4pt) *Text measurement on `Canvas`.* Measurement
  existed only on the painter, not on the canvas interface handed to a custom
  painter — so a widget drawing its own chart or gauge could not ask how wide a
  label was before placing it. The three measurement methods are now part of the
  `Canvas` contract, implemented by both the painter-backed canvas and the scene
  recorder, and both answer from the same painter that will shape the run, so a
  measurement and the draw that follows it cannot disagree.

  It is listed here because it is evidence about the gap list as a whole: the
  interface shape already existed internally, and the change was additive. Most
  of this section is of that character.
]

== Platform and integration

#datatable(
  (auto, 1fr, auto),
  th[Gap], th[Detail], th[Rank],
  table.hline(stroke: 0.5pt + rule),

  [Native platform views], [Embedding a native subview — a map, a video player,
    an advertisement, an in-application browser — in the widget tree is
    unimplemented on native targets by explicit decision. The reasoning given
    (authentication belongs in the platform's dedicated browser session) is
    correct for authentication and does not cover the rest. Platform views and
    their Compose equivalent are load-bearing for a large class of commercial
    applications. Two qualifications found on closer inspection, both of which
    lower this item's priority: the in-tree list of what the decision blocks
    overstates its leverage by six of eight entries and is simply wrong about a
    seventh — biometrics already ships — leaving *video playback* as the only
    capability genuinely waiting on it; and on mobile the work needs no change
    to the vendored fork at all. On desktop the native handle is five lines
    behind an unexported field, so the real cost there is splicing a native
    subtree into three accessibility backends, not reaching the window. Two
    things are genuinely impossible rather than expensive: a native view cannot
    be interleaved between two widget layers, and it can never appear in an
    offscreen snapshot — which makes this the one feature that erodes the
    project's headless-testing advantage.], chip(blocking, "Blocking"),

  [No third-party extension point], [The capability generator parses the
    framework's own source as its single source of truth, so nobody outside the
    repository can ship a package that adds a capability without patching the
    tree. This is precisely the mechanism behind Flutter's package registry, and
    its absence caps the ecosystem at whatever the core team writes.],
    chip(blocking, "Blocking"),

  [Hot reload], [There is none, on any platform, and there cannot be. This is
    foreclosed by *Go*, not by Apple: `buildmode=plugin` requires CGo — which
    this project does not have — and is unsupported on both `android/arm64` and
    `ios/arm64` at any CGo setting, so there is no dynamic code loading to build
    on. (The common "iOS forbids dynamic code loading" argument is too strong
    for the case that matters: Flutter's iOS *debug* builds run the Dart JIT,
    which Apple permits. Only distribution builds are AOT.) What the project
    ships instead is state-preserving hot *restart* — rebuild, relaunch,
    rehydrate — on *every* target: desktop, web, the Android emulator and the
    iOS simulator, all driven by one `widget.StateSnapshot` keyed by position in
    the widget tree. Measured steady-state iteration on an Apple Silicon Mac:
    1.4~s desktop, ~4~s web, ~11~s on an Android emulator and ~20~s on a
    pre-booted iOS simulator, against roughly a second for a Flutter reload
    anywhere. `gomobile bind` is 60--70% of the mobile figure and is not
    incrementally rebuildable, so that gap closes only by owning the bind. This
    is the one item on the list that was closed by renaming it accurately rather
    than by reaching parity — and "hot restart on every target, including a
    phone" is a claim no competitor makes about on-device Go.],
    chip(notable, "Notable"),

  [Desktop packaging], [The command-line tool builds a binary but produces no
    application bundle, disk image, installer, AppImage, or notarisation. Mobile,
    by contrast, is scaffolded thoroughly — generated Xcode and Gradle projects
    with Kotlin and Swift hosts — which makes desktop the conspicuous omission.],
    chip(notable, "Notable"),

  [Internationalisation depth], [29 locales with hand-written plural rules. No
    ordinal plurals, no relative time formatting, no locale-aware currency or
    number patterns, and no message extraction tooling. Flutter ships a full ICU
    pipeline; Compose inherits Android's complete locale data.],
    chip(notable, "Notable"),
)

== Ecosystem, stability and adoption

This is the section that engineering effort alone cannot close, and it is where
the real distance to the incumbents lies.

#datatable(
  (auto, 1fr, auto),
  th[Gap], th[Detail], th[Rank],
  table.hline(stroke: 0.5pt + rule),

  [No stability commitment], [Version `v0.1.0`, no compatibility promise. The
    generated API manifest records names and kinds *but not signatures*, so a
    signature-only break can pass a green build. For a framework, the
    compatibility guarantee is the product.], chip(blocking, "Blocking"),

  [No package ecosystem], [Zero third-party packages. The incumbents win most
    evaluations on "there is a package for that" rather than on architecture.],
    chip(blocking, "Blocking"),

  [Documentation depth], [A documentation site, a live gallery and an agent-facing
    skill exist. Absent: an API reference tour, a cookbook of recipes, migration
    guidance, and a structured learning path.], chip(notable, "Notable"),

  [No frame-budget policy], [Allocation guards exist in benchmarks, but there is
    no garbage-collection tuning or documented per-frame allocation budget
    anywhere in the tree. Go's pauses are short, and sustained allocation churn
    at 120~Hz is still the obvious latency risk. This is a gap in
    *measurement*, not a demonstrated defect.], chip(minor, "Minor"),
)

// ─────────────────────────────────────────────────────────── 6

= Competitive assessment

#set text(size: 9pt)
#table(
  columns: (1fr, 1fr, 1fr, 1fr),
  stroke: none,
  inset: (x: 5pt, y: 6pt),
  align: left + top,
  fill: (_, y) => if calc.odd(y) { wash },
  table.header(th[Dimension], th[Gophics], th[Flutter], th[Compose Multiplatform]),
  table.hline(stroke: 0.5pt + rule),
  [Language], [Go], [Dart], [Kotlin],
  [Toolchain], [`go build`], [Full SDK + engine], [Gradle + Kotlin Multiplatform],
  [Rendering], [Own, pure-Go WebGPU], [Own (Impeller/Skia)], [Own (Skia)],
  [Desktop a11y], [All three platforms], [Weak], [JVM-dependent],
  [Hot reload], [Impossible in Go], [Best in class], [Strong],
  [Hot restart, state kept], [Every target, phones included], [Yes], [Yes],
  [Platform views], [Absent on native], [Mature], [Mature],
  [Ecosystem], [None], [~50,000 packages], [JVM + Android ecosystem],
  [Headless testing], [First-class], [Limited], [Limited],
  [On-device test binaries], [Yes], [No], [No],
  [Terminal target], [Yes], [No], [No],
  [Web payload], [2.73 MB Brotli], [Comparable], [Large],
  [Stability], [`v0.1.0`], [Stable, versioned], [Stable, versioned],
)
#set text(size: 10pt)

#verdict("The strategic reading")[
  Gophics loses on ecosystem, mobile iteration speed and stability guarantees. It
  wins on toolchain weight, testability, desktop accessibility, dependency
  surface, and deployment simplicity. Those are not symmetrical quantities: an
  ecosystem is accumulated over years by thousands of contributors, whereas a
  single static binary with no SDK is a property a competitor *cannot* retrofit.

  Competing as "Flutter, but in Go" means losing on the ecosystem axis
  permanently. Competing on single-binary deployment, auditability, headless
  testing and Go-native integration means most of #link(<gaps>)[the Tier 2 list]
  stops being decisive — because the buyers who care about those properties are
  not building consumer applications with embedded maps and advertisements.
]

== Where the wedge is

Four segments where the trade-offs favour Gophics today, before any gap is
closed:

// Unbreakable: this table is four rows, and letting it split left a single
// row stranded on a page of its own ahead of the next section's pagebreak.
#block(breakable: false)[
#datatable(
  (auto, 1fr),
  th[Segment], th[Why Gophics wins],
  table.hline(stroke: 0.5pt + rule),
  [Internal and operational tools], [Deployment is a file copy. No SDK on the
    build machine, no app store, and accessibility is already compliant.],
  [Developer tooling], [A Go team ships a GUI without adopting a second
    language or a second build system. A live interface can be embedded in a
    server or rendered to a PNG headlessly.],
  [Embedded, kiosk and industrial displays], [A single static binary with a
    software rendering fallback, cross-compiled to the target, auditable to nine
    dependencies.],
  [Regulated and air-gapped environments], [Nine dependencies and no CGo is a
    supply chain a security team can actually review; the absence of an engine
    artifact removes an entire class of provenance questions.],
)
]

// ─────────────────────────────────────────────────────────── 7

= Recommendation

== Sequenced work <gaps>

Ordered by value delivered per unit of risk, not by size.

#set text(size: 9pt)
#table(
  columns: (auto, 1fr, auto, auto),
  stroke: none,
  inset: (x: 5pt, y: 6.5pt),
  align: left + top,
  fill: (_, y) => if calc.odd(y) { wash },
  table.header(th[#sym.numero], th[Action], th[Effort], th[Risk]),
  table.hline(stroke: 0.5pt + rule),
  [1], [*Record signatures in the API manifest, then commit to `v1` on a defined
        surface.* The cheapest item on this list and the one that unblocks every
        adoption conversation. A framework without a compatibility promise cannot
        be chosen by anyone who would have to maintain the result.],
       [Low], [None],
  [2], [*Promote the existing spring solver out of the scroll implementation into
        the animation package and add velocity handoff.* The difficult part is
        written and tested; it is currently unreachable.], [Low], [None],
  [3], [*Expose blend modes and multi-stop, radial and sweep gradients.* The
        underlying context already accepts a blend mode on every call, and
        gradients are already approximated with clipped strips — this is
        extension of working code.], [Medium], [Low],
  [4], [*Define a third-party capability extension point.* The only item with
        compounding returns, because it is the sole mechanism by which other
        people close gaps on your behalf.], [Medium], [Medium],
  [5], [*Colour emoji.* Highest user-visible impact. Requires settling how colour
        glyphs reach the surface without diverging the vendored graphics fork —
        see below.], [High], [Medium],
  [6], [*Desktop packaging and web font fallback.* Packaging is a
        well-understood chore. Web fonts is not: the browser cannot supply font
        bytes, so the deliverable is on-demand fetching plus an honest
        diagnostic, not parity with the native path.], [Medium], [Low],
  [7], [*Native platform views, full locale data, and a faster mobile bind.*
        Expensive. The bind is the whole of the mobile iteration loop and
        shortening it means owning a step `gomobile` owns today; start only
        with a number from a real developer's hands. Sequence according to the
        chosen wedge rather than to competitive symmetry.], [High], [Medium],
)
#set text(size: 10pt)

== The one decision that cannot be deferred

Colour emoji is the most visible defect and the only item in Tier 1 that is a
project rather than a task. It needs a direction chosen before work starts,
because the three routes have very different consequences:

#datatable(
  (auto, 1fr),
  th[Route], th[Consequence],
  table.hline(stroke: 0.5pt + rule),
  [Accept a pipeline flush per emoji run],
  [*Cheapest, and now measured: there is no flush.* The warning was stale — it
   predates the substrate being vendored, and the image primitive queues into
   the same ordered list as shapes and text, so a mid-frame image draw drops
   nothing on any path. Cost is 26--33~µs per interleaved emoji, or 0.9% of a
   60~Hz frame for five.],

  [Deferred image queue composited in-pass],
  [Architecturally correct and keeps the vendored fork untouched. Moderate work
   inside the paint layer, which is first-party code.],

  [Add an RGBA glyph atlas tier to the graphics substrate],
  [Fastest result at runtime, but diverges a vendored fork that the project's own
   guidance says to leave alone. A maintenance liability in perpetuity.],
)

#verdict("Recommended")[
  *Route (a), and neither of the others.* The contradiction has since been
  resolved by measurement and it collapsed the decision: the warning was stale,
  no flush occurs, and the deferred queue's own ceiling is already visible in
  the measurements as the non-interleaved case — about 0.1~ms on a five-emoji
  paragraph, bought by re-implementing the substrate's ordered draw list one
  layer up and inventing a second z-order invariant to maintain. Not worth it.
  Do not diverge the graphics fork either.

  Two findings displaced the flush question as the real work, and both would
  have been missed by costing the routes alone. Font fallback never reaches an
  emoji face at all, so fixing glyph emission changes nothing on its own; and
  one texture per glyph falls off a cliff at 65 distinct images against the
  substrate's 64-texture cache, so the glyphs must be blitted from a single
  atlas. A third is load-bearing and quieter still: ink bounds are computed from
  outlines only, so an emoji contributes zero extent and a label consisting of
  one emoji draws nothing — silently — even after the other two are fixed.

  One scope limit belongs in the decision rather than in the implementation.
  Colour fonts are checked for a `COLR` paint graph before any bitmap table, so
  no bitmap route fixes Windows' Segoe UI Emoji. Colour emoji on Windows is a
  separate tier and should be named as a non-goal now rather than discovered
  later.
]

== What to decide now

#datatable(
  (auto, 1fr),
  th[Decision], th[Why it is yours and not an engineer's],
  table.hline(stroke: 0.5pt + rule),
  [Which wedge], [Everything in Tier 2 is either essential or irrelevant
    depending on the answer. Consumer applications need platform views and hot
    reload; internal tools and embedded displays need neither.],
  [When `v1`], [The stability promise gates adoption more than any feature on
    this list, and it constrains API work from the moment it is made.],
  [Whether to court contributors], [If yes, the extension point and the
    documentation move up the list ahead of rendering work, because they are
    what make outside effort possible.],
)

// ─────────────────────────────────────────────────────────── Appendix

#set heading(numbering: "A.1")
#counter(heading).update(0)

= Appendix A — Method and limits

== How claims here were established

Every quantitative statement was taken from the source tree at commit
`dbafdb5`: package and line counts by enumeration, test and benchmark counts by
pattern match across the tree, artifact sizes by compiling the counter example
and compressing the output, and the capability matrix by reproducing the file
that the project's own build gate verifies. Behavioural claims — colour emoji,
web font fallback, gradient and blend-mode support, animation primitives,
accessibility backends, platform views — were established by reading the
implementing code rather than its documentation.

== What was not verified

Stated plainly, because a white paper that does not distinguish measurement from
inference is not useful.

#datatable(
  (auto, 1fr),
  th[Not verified], th[Why],
  table.hline(stroke: 0.5pt + rule),
  [Runtime behaviour on real devices], [No iOS or Android hardware was exercised
    during this assessment. The Windows virtual machine was unreachable.],
  [Sustained frame-rate and latency under load], [No profiling was run. The
    frame-budget observation is about the absence of a documented policy, not a
    measured regression.],
  [Competitor figures], [Flutter and Compose characteristics are stated from
    general knowledge and are not measured here. Only the Gophics numbers in this
    report were taken directly.],
  [Visual quality], [No side-by-side comparison against native widgets was made,
    although the repository contains a harness built for exactly that.],
)

== A note on the gap list

The gap list shortened materially while it was being compiled. Two items
initially recorded as missing — implicit animations and Windows accessibility —
turned out to be implemented, and implemented well. A third, text measurement on
the canvas, was closed in the course of the assessment. The pattern is
consistent: this codebase is further along than its surface area suggests, and
the binding constraints are positioning, stability and ecosystem rather than
engineering capability.
