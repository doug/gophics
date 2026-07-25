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

# the same todo app in the browser:
GOOS=js GOARCH=wasm go build -o examples/todo/web/todo.wasm ./examples/todo
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/todo/web/
python3 -m http.server -d examples/todo/web 8080   # open http://localhost:8080
```

Packages:

- `geom` — points, rects, rrects, insets, affine transforms (float32,
  logical pixels — ADR 0001)
- `shell` — the platform interface: windows, frames, input events
- `shell/desktop` — macOS/Linux/Windows shell over gogpu (pure Go, no CGo)
- `shell/web` — browser shell (js/wasm): canvas presentation, RAF frames,
  DOM input
- `paint` — Canvas interface; gg CPU rasterizer backend, GPU-composited on
  desktop, pixel-presented on web; gradients, drop shadows, offscreen
  rendering for golden tests
- `text` — shaping, bidi, font fallback, UAX #14 line breaking
  (go-text/typesetting), glyph outline extraction
- `scene` — recorded display lists, diffed for damage, replayable onto
  any Canvas
- `anim` — curves and controllers driven by frame tickers
- `layout` — the box protocol (constraints down, sizes up) + core boxes:
  flex, padding, align, sized, decorated, text
- `widget` — immutable widgets, keyed reconciliation into a retained
  element tree, `StateBase[W]` generics state, interaction handlers
- `app` — the runtime: `app.Run` for a window, `app.Headless` for
  widget tests without a display
