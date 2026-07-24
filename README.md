# gossamer

A Flutter-class UI framework in pure Go — Flutter's pipeline architecture
(immutable widgets → element reconciliation → constraint layout → layer
compositing), reimagined with idiomatic Go APIs, on a zero-CGo GPU stack
(gogpu/wgpu). Desktop, web (WASM), and eventually mobile.

**Status: M2/M3 core landed** (layout protocol + widget/element trees).
See [PLAN.md](PLAN.md) for the full plan, architecture, and roadmap;
[docs/adr/](docs/adr/) for decisions.

```sh
go run ./examples/hello   # a colored, vsynced, resizable window
go run ./examples/todo    # a widget-tree todo app: state, taps, hover, text input
```

Packages:

- `geom` — points, rects, rrects, insets, affine transforms (float32,
  logical pixels — ADR 0001)
- `shell` — the platform interface: windows, frames, input events
- `shell/desktop` — macOS/Linux/Windows shell over gogpu (pure Go, no CGo)
- `paint` — canvas over gg's CPU rasterizer, GPU-composited; offscreen
  rendering for golden tests
- `layout` — the box protocol (constraints down, sizes up) + core boxes:
  flex, padding, align, sized, decorated, text
- `widget` — immutable widgets, keyed reconciliation into a retained
  element tree, `StateBase[W]` generics state, interaction handlers
- `app` — the runtime: `app.Run` for a window, `app.Headless` for
  widget tests without a display
