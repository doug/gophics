# Gophics — feature roadmap

What to build next, and *why*. The ordering is by what unblocks real use:
finish the things that stop someone shipping an app, then close the gaps that
make it pleasant, then go deeper. Desktop, web and embedded use come first;
consumer-mobile polish is sequenced last, because keeping mobile *working* is
cheap and making it app-store-grade is not.

---

## Recently shipped

- **v0.1.0 tagged** — first release; the API surface is now versioned.
- **Zero-CGo capability layer** — clean `ctx.<Cap>()` Go interfaces + `internal/capgen`
  generated wiring for 15+ platform services (FilePicker, Share, Notifier,
  SecureStorage, Clipboard, Connectivity, Battery, Lifecycle, Links, Gamepad,
  Geolocation, TextInput/IME, Accessibility, WebView, Socket). Web impls
  **browser-verified** via `examples/capabilities` (2026-08-10); native leaves are
  honest `TODO(platform)` fill-ins. See `docs/design-capabilities.md`.
- **GPU opacity-layer compositing** — Skia-style `saveLayer` offscreen compositing,
  headless-Metal + on-device (Pixel 10 Pro) verified (`design/gpu-opacity-layers.md`).
- **Substrate consolidation** — seven GPU/audio forks vendored in-tree; one module,
  no go.work, no `replace`, zero CGo (`design/substrate-consolidation.md`).

The capability layer makes platform services a uniform Go interface that degrades
gracefully where a host provides nothing. It is not its own roadmap line because
the pattern is done; what remains is per-platform native leaf work, sequenced
with desktop/mobile below.

---

## P0 — now

1. **Shrink the web payload.** Go compiles the whole renderer into a ~14–20 MB
   WASM blob (~5 MB gzipped). "One binary, desktop **and** web" is only true in
   spirit if the web half is that heavy. Attack: `-ldflags=-s -w` (done),
   aggressive dead-code elimination, a **CPU-only lean web renderer build** that
   drops the WebGPU/naga/shader path for demos that don't need it, `brotli` on
   Pages, and lazy/streamed instantiation. *Goal: a flagship demo under ~2–3 MB
   gzipped, and a documented note on what the size is made of.*
2. **Make headless testing a first-class feature.** Ship a golden/snapshot
   harness: image capture + diff with tolerance, a one-command update flow, and
   assert-on-the-widget/semantics-tree helpers. Document it with a copy-paste
   example. Rendering a widget tree to a PNG in CI with no display already works;
   this makes it something you can reach for without reading the internals.
3. **The docs site + live demo gallery.** One URL that runs the same app natively
   and in-browser (WebGPU), beside a `go test` that asserts on it.
   *(In progress — `docs/`.)*
4. **The hot-reload substitute.** Sub-second edit→see-it is structurally
   unavailable in Go (PLAN §6.3). Deliver the closest honest thing: fast rebuild
   + opt-in **state snapshot** ("hot restart that remembers", already spiked) +
   **headless preview** (re-render a subtree to a browser tab on save).

## P1 — next

5. **The embeddable-library story, told well.** Runnable examples + docs for the
   programs that are awkward everywhere else: a **server with a live
   admin/dashboard panel**, a **CLI that pops a window**, a **pipeline with a viz
   pane**.
6. **Widget breadth + a polished default theme.** What dashboards and tools
   actually need: a **data table / grid**, richer **forms** (date/number/select),
   **menus/command palette**, **tabs**, **tooltips**, and a clean default design
   language so apps look good with zero styling. Unblocked work.
7. **Native-feel scrolling & input polish.** Momentum/rubber-band/trackpad to
   macOS quality (queued), pointer cursors, keyboard/focus scopes.

## P2 — later

8. **Accessibility completion.** Finish the a11y bridges (on-device VoiceOver
   validation; the `accesskit_c` purego binding). Semantics is already testable
   headless.
9. **GPU vector backend (Vello-style sparse strips).** Performance ceiling for
   heavy scenes; the CPU backend + gg accelerator carry until then. Measured
   against the render-pass path it would replace, it is slower in every scene
   tested so far — see `design/milestones.md`.
10. **Consumer-mobile polish.** Sequenced last on purpose — GPU present is proven
    on iOS + Android, but app-store-grade mobile polish is a design-team-scale
    investment. Keep mobile *working*, invest elsewhere first.

## Explicit non-goals

- **Pixel-cloning another toolkit's design language.** One clean default; theming
  is a later exercise, not a parity exercise.
- **Hot-reload parity with a dynamic VM.** Structurally impossible in Go; the P0
  substitute is the answer.
- **App-store-grade consumer mobile, for now.** Desktop tools, embedded UIs,
  local-first apps and server-side rendering come first.

## If we do only one thing this quarter

**Land P0 #1 + #3 together: a lean, fast web demo shipped in the docs site.** The
architecture is finished; a demo that loads quickly is both how people find it
and the proof that the claim holds. P0 #2 is the close second.
