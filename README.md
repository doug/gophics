# gossamer

A Flutter-class UI framework in pure Go — Flutter's pipeline architecture
(immutable widgets → element reconciliation → constraint layout → layer
compositing), reimagined with idiomatic Go APIs, on a zero-CGo GPU stack
(gogpu/wgpu). Desktop, web (WASM), and eventually mobile.

**Status: M0 (spikes & skeleton).** See [PLAN.md](PLAN.md) for the full
plan, architecture, and roadmap; [docs/adr/](docs/adr/) for decisions.

```sh
go run ./examples/hello   # a colored, vsynced, resizable window
```

Packages so far:

- `geom` — points, rects, rrects, insets, affine transforms (float32,
  logical pixels — ADR 0001)
- `shell` — the platform interface: windows, frames, input events
- `shell/desktop` — macOS/Linux/Windows shell over gogpu (pure Go, no CGo)
