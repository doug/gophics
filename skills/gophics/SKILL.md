---
name: gophics
description: >-
  Write or edit Go apps that build cross-platform native UIs with gophics
  (github.com/doug/gophics). Use whenever a task involves gophics widgets,
  app state, layout, or custom canvas drawing — it teaches the mental model
  and idioms so you don't reach for React/Flutter/Dart patterns that don't
  exist here.
---

# Building UIs with gophics

Gophics is a **pure-Go**, cross-platform UI framework: one widget tree renders
the same on desktop, web (WASM/WebGPU), iOS, Android, and a terminal. It borrows
the architecture of Flutter — immutable widgets, a reconciling element tree,
constraint layout — but the API is idiomatic Go. There is **no codegen, no DSL,
no Dart, no JSX**. Widgets are plain structs.

Import root: `github.com/doug/gophics`. Build with `CGO_ENABLED=0`.

## The mental model (five ideas)

1. **A widget is an immutable struct value.** You describe the UI by
   constructing structs (`widget.Text{...}`, `widget.Column(...)`). You never
   mutate a widget after building it — you build a fresh tree each frame.
2. **Two kinds of widget.** *Stateless* — implements `Build(ctx) Widget`, builds
   from its fields alone. *Stateful* — implements `CreateState() State`; the
   returned State holds mutable data and has its own `Build`.
3. **State changes go through `SetState`.** Mutate state fields only inside
   `s.SetState(func(){ ... })`; that schedules a rebuild. Mutating outside it
   won't repaint.
4. **`Build` returns a tree of child widgets.** Compose with layout primitives
   (`Column`, `Row`, `Padding`, `Stack`, …). Composition, not inheritance.
5. **`app.Run(root, cfg)` starts the app.** `root` is any widget; `cfg` sets the
   window title, size, background, and font.

## Minimal app — state + interaction

A complete counter. (Compiled, kept honest by CI: see
`examples/counter/main.go` next to this file.)

```go
type Counter struct{} // the widget: immutable config (here, none)

func (Counter) CreateState() widget.State { return &counterState{} }

type counterState struct {
	widget.StateBase[Counter] // embeds W() and SetState(); NOTE: no Context()
	n int
}

func (s *counterState) Build(ctx widget.Ctx) widget.Widget {
	return widget.Column(
		widget.Text{S: fmt.Sprintf("count: %d", s.n), Size: 28, Color: paint.RGB(0.92, 0.93, 0.95)},
		widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { s.SetState(func() { s.n++ }) }},
			Child:   widget.Text{S: "increment", Size: 18, Color: paint.RGB(0.36, 0.62, 0.98)},
		},
	)
}

func main() {
	app.Run(Counter{}, app.Config{
		Title: "counter", Size: geom.Size{W: 320, H: 200},
		Background: paint.RGB(0.07, 0.08, 0.11), Font: goregular.TTF,
	})
}
```

Key facts from that example:
- `widget.StateBase[Counter]` gives the state `W() Counter` (current config) and
  `SetState(func())`. It does **not** provide a `Context()` — the build context
  arrives as the `ctx widget.Ctx` argument to `Build`.
- `Font` is **required** for any text to render (`Config.Font` is `[]byte` of a
  TTF/OTF; `golang.org/x/image/font/gofont/goregular` is the usual default).

## Composing UI — layout primitives

These are the load-bearing ones. All live in package `widget`.

| Primitive | Shape |
| --- | --- |
| `Column(children...)`, `Row(children...)` | flex stacks (return a `Flex`) |
| `Flex{Direction, Justify, Align, Children}` | full flex control |
| `Expand(child)` / `Flexible{Flex, Child}` | grow to share free space |
| `Spacer()` | flexible empty gap |
| `Padding{All: 8, Child: …}` or `Padding{Insets: …}` | inset a child |
| `Center(child)` / `Align{X, Y, Child}` | position within the parent |
| `Sized{W, H, Child}` | fixed dimensions (0 = unspecified) |
| `Fill{Color, Child}` | fill available space, paint a background |
| `Stack{Children}` | overlap / z-stack |
| `Scroll{Child}` | scrollable viewport |
| `LazyList{Count, Item}` | virtualized long lists |
| `Text{S, Size, Color, Wrap}` | text |
| `Interactive{Handler, Child}` | make a child respond to input (adds no visuals) |
| `WithKey{Key, Child}` | stable identity for list reconciliation |

`Handler` carries `OnTap`, `OnEnter`, `OnExit`, `OnPress(pos)`, `OnDrag(pos, delta)`.

## Custom drawing — the escape hatch

For graphics that don't decompose into widgets (charts, gauges, game boards,
generative art), use `widget.Canvas`. Its `Draw` gets a `paint.Canvas`:

```go
widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
	c.Clear(paint.RGB(0.09, 0.10, 0.13))
	c.FillRRect(geom.RectXYWH(20, 20, 120, 80), 8, paint.RGB(0.36, 0.62, 0.98))
	c.Text("hello", geom.Pt{X: 30, Y: 60}, 16, paint.RGB(1, 1, 1))
}}
```

`paint.Canvas` primitives: `Clear`, `FillRect`, `FillRRect`, `FillRRectGradient`,
`StrokeRRect`, `FillPath`, `StrokePath`, `Line`, `Text`, `Image`, `DrawSprite`,
and stack ops `PushClip`/`PopClip`, `PushOpacity`/`PopOpacity`,
`PushTransform`/`PopTransform`. See `examples/customdraw/main.go` beside this file.

## Colors & geometry

- `paint.RGB(r, g, b)` → opaque `Color` (components in [0,1]). For alpha, use a
  literal `paint.Color{R, G, B, A}` or `someColor.WithAlpha(a)`. `paint.Lerp(a,b,t)`.
- `geom.Size{W, H}`, `geom.Pt{X, Y}`, `geom.RectXYWH(x, y, w, h)`, `geom.Insets{...}`.

## Gotchas (the durable ones)

- **Never mutate a built widget** or a state field outside `SetState`. Widgets
  are values; rebuild instead of mutating.
- **State lives in the State struct, not the widget struct.** The widget struct
  is immutable configuration passed in from the parent.
- **Text needs a font** (`Config.Font`) or nothing draws.
- **`CGO_ENABLED=0`** — the whole stack is zero-CGo; don't add cgo deps.
- **Keyed lists:** when a list reorders/inserts, wrap items in `WithKey` so the
  reconciler preserves state and animations.
- **Don't invent widgets.** The catalog is intentionally small and composed from
  primitives. If you need a "Button" or "Card", build it from `Fill` + `Padding`
  + `Interactive` + `Text`.

## Discovering more (authoritative sources — prefer these over guessing)

The code is the source of truth. When unsure whether an API exists or what its
fields are, **check** rather than guess:

- `go doc github.com/doug/gophics/widget` — the full widget catalog.
- `go doc github.com/doug/gophics/widget SomeType` — one type's fields/methods.
- `go doc github.com/doug/gophics/paint`, `.../geom`, `.../app` — the rest.
- Read real usage in the repo's **`examples/`** directory (hello, todo, hn,
  notes, canvas, solitaire, …) — these compile and are the best patterns.

## Version

Written against `github.com/doug/gophics` at commit `66340ac` (2026-08-04). The
project is young and moving fast; **treat `go doc` and the `examples/` in the
version pinned in your `go.mod` as authoritative** if anything here disagrees.
The compiled example files and `apicheck/` package beside this skill fail CI if
an API named here is renamed or removed.
