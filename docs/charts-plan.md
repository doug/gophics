# A built-in chart library for gossamer

## Goal

A first-party, **Swift Charts–style** charting package — declarative marks composed
over scales, rendered through `paint.Canvas`, animated with `anim.Controller`, and
theme-/accessibility-aware. Pure standard library plus gossamer's own layers
(`paint`, `widget`, `geom`, `anim`, `layout`) — **zero new external dependencies**.
"Kitchen sink, batteries included": a habit heatmap, a budget trend, a workout
progress line, or a stock candlestick should each be a dozen lines of app code.

Reference for the *feel* (not the API): Apple's Swift Charts —
`Chart { BarMark(x:, y:) }`. We adapt the declarative-marks idea to idiomatic Go
(typed mark structs, not a macro DSL).

The design test applied throughout, borrowed from `docs/games-plan.md`: **the `chart`
package holds only conveniences over `paint`/`widget`; any new *primitive* goes into
`paint` and must earn its place from non-chart callers too.**

---

## Where the framework stands (verified against the code, not assumed)

**Reachable today, and enough for most chart types:**

- **Text measurement is available.** `Ctx.Painter()` (`widget/widget.go:94`) returns
  the `*paint.Painter`, which has `MeasureWidth`/`MeasureWidthIn`/`Metrics`/`Shape`
  (`paint/paint.go:386–414`). A chart stashes the painter in `Init`/`Build` and sizes
  its axis gutters from the **widest tick label** before drawing. So proper axis
  margins, right-aligned y-labels, and rotated/elided x-labels are all doable now —
  this was the feared gap and it isn't one.
- **Marks that need only rectangles, lines, gradients and text:** `FillRect`,
  `FillRRect`, `FillRRectGradient`, `StrokeRRect`, `Line`, `Text`/`TextIn`, `Image`,
  plus `PushClip`/`PushClipRRect` (clip to plot area) and `PushTransform` (rotate an
  x-axis label). This covers **bar, column, line (per-segment), point/scatter, rule
  (reference lines), rect (heatmap/matrix), range (min–max bars), and candlestick.**
- **Animation:** `anim.Controller` + curves (proven in `examples/solitaire`). Charts
  interpolate old→new mark values so bars grow and lines draw in.
- **Interaction:** `widget.Interactive{Handler{OnTap,OnDrag,OnPress}}` around the plot
  Canvas → pixel-to-datum hit-testing → selection state → a tooltip overlay via
  `widget.Stack`/`Align`.
- **Theme + a11y for free:** `Ctx.DarkMode()`, `Ctx.ReduceMotion()`,
  `Ctx.SafeInsets()` — palettes flip for dark mode, animation is skipped under
  reduce-motion.
- **Testing:** `app.NewHeadless` golden images (the whole `examples/*` test pattern),
  so every chart type gets a deterministic pixel test.

**The one real gap — path/polygon fill.** `paint.Canvas` has no filled path or
polygon; `FillRRect(square, radius=half)` fakes a circle, but there is no way to fill
an arbitrary closed shape. That gates exactly four things:

| Mark | Needs |
|---|---|
| **Area** (filled under a line) | fill a closed polygon (line + baseline) |
| **Pie / Donut** (`SectorMark`) | fill a wedge (arc + two radii) |
| **Smoothed line** (Catmull-Rom/monotone) | stroke a curved path with joins |
| **Thick line with crisp joins** | `Line` is per-segment → miter gaps at vertices |

This is the **same primitive `docs/games-plan.md` §Workstream 3 already specs**:
`paint.Path` + `FillPath(p, rule, c)` / `StrokePath(p, style, c)` (the fork's
`gg.Path`/`Context.FillPath` already exist; only the `paint.Canvas` exposure is
missing). **Charts and games share it — building it serves both**, which is exactly
the "earns its place beyond one caller" test. So:

- **Charts v1 ships every mark that needs only rects + lines + measured text.**
- **v2 unlocks area / pie / donut / smoothed lines the day `paint.Path` lands.**
- Interim hacks (area as a clipped vertical gradient under a polyline; pie as a
  many-wedge `FillRect` fan) are ugly enough to skip — defer to the real primitive.

Optional, minor: a device-pixel-ratio accessor (`Ctx.DevicePixelRatio()`, also on the
games-plan wishlist) would let charts snap gridlines to physical pixels for hairline
crispness. Nice-to-have, not a blocker.

---

## Package shape — `chart`

Idiomatic-Go declarative: a `Chart` **is a `widget.Widget`**; you hand it typed marks
and let scales/axes/legend infer from the data.

```go
package chart // deps: paint, widget, geom, anim, layout, std only

// A Chart is a widget: plot area (Canvas) + measured axes + legend + interaction.
type Chart struct {
    Marks  []Mark            // drawn back-to-front
    X, Y   Scale             // optional; inferred from mark data if nil
    XAxis  Axis              // ticks, label formatter, grid on/off
    YAxis  Axis
    Legend Legend            // auto from series names/colors
    Frame  layout.Insets     // plot inset overrides (else measured from labels)
    Select *Selection        // nil = non-interactive; else hover/tap → tooltip
    Animate bool             // interpolate on data change (respects ReduceMotion)
}

// Mark is one visual layer. Each maps its data through the scales into the plot rect.
type Mark interface {
    domainX() (lo, hi float64, band []string) // for scale inference
    domainY() (lo, hi float64)
    plot(p Plot)                              // draw into p.Canvas within p.Area
}

// Plot is what a mark draws against: the pixel rect + the resolved scales + painter.
type Plot struct {
    Area   geom.Rect
    X, Y   Scale
    Canvas paint.Canvas
    Meas   *paint.Painter // measurement for in-plot labels
    T      float32        // 0..1 animation progress (1 when settled)
}
```

**Marks (v1 — ship on today's primitives):**

```go
type BarMark   struct{ Data []Datum; Color paint.Color; Corner float32; Horizontal bool }
type LineMark  struct{ Data []Datum; Color paint.Color; Width float32; Points bool }
type PointMark struct{ Data []Datum; Color paint.Color; Size float32; Shape PointShape }
type RuleMark  struct{ At float64; Axis AxisKind; Color paint.Color; Dash []float32 } // targets/thresholds
type RectMark  struct{ Cells []Cell; Scale ColorScale }                              // heatmap / calendar
type RangeMark struct{ Data []Span;  Color paint.Color }                             // min–max, error bars
// (candlestick composes RangeMark + BarMark)
```

**Marks (v2 — on `paint.Path`):**

```go
type AreaMark   struct{ Data []Datum; Fill paint.Color; Line paint.Color; Stack bool }
type SectorMark struct{ Data []Datum; Inner float32 } // pie (Inner=0) / donut
// LineMark.Smooth bool becomes meaningful (monotone-cubic) once StrokePath exists.
```

**Data**: a `Datum{X, Y float64; Label string; Color paint.Color}` (zero Color =
series default). Categorical x uses `Datum.Label` with a **band scale**. This keeps
each mark self-contained (a slice), sidestepping a data-frame abstraction we don't
want.

**Scales** (pure, unit-tested, no rendering):

```go
type Scale interface {
    Map(v float64) float32     // domain → [0,1] within the axis
    Invert(t float32) float64  // pixel-fraction → domain (for hit-testing/tooltips)
    Ticks(target int) []Tick   // "nice" tick selection
    Domain() (lo, hi float64)
}
// Linear, Log, Time (calendar-aware "nice" ticks), Band (categorical), plus a
// ColorScale (sequential/diverging) for heatmaps.
```

**Axis / Legend / Selection** are small structs: `Axis{Show, Grid, Ticks int,
Format func(float64) string, LabelAngle float32}`; `Legend{Show, Position}`;
`Selection{Index int; Datum Datum; ...}` filled by hit-testing `Scale.Invert`.

**Rendering path.** `Chart.Build` (a) stashes `ctx.Painter()`, (b) measures the
widest y-label and tallest x-label to compute the plot inset, (c) returns
`Stack{ Canvas(plotAndAxes) , tooltipOverlay }` wrapped in `Interactive`. The Canvas
draw closure resolves scales for the current `Area`, draws grid → axes → marks
(back-to-front) → selection highlight, reading animation `T` from a `Controller`.
**No widget-per-bar** — one Canvas, bespoke layout, exactly as `examples/solitaire`.

**Animation.** On data change the Chart lerps each datum old→new (`T` via an
`EaseOut` `Controller`); marks read `T` to grow/draw-in. Skipped when
`ctx.ReduceMotion()`.

**Accessibility (named, deferred like solitaire's).** A Canvas plot is invisible to
the a11y tree; mitigation is a later transparent `widget.Semantics` overlay
summarizing series/values — the same debt and the same fix as the solitaire board.

---

## The example app — a data dashboard that needs the whole library

The forcing function should genuinely exercise most marks, be local-first and
on-thesis (own-your-data, replaces a subscription), and read as a real app.

### Primary pick: **Ledger** — a personal-finance / budget dashboard

Import transactions (CSV/OFX) from a local file (File System Access on web, reusing
the `examples/notes` store pattern), then *visualize* them. It's the "insights half"
of YNAB/Actual — deliberately not a full budgeting engine — and it lights up nearly
every mark:

| Screen | Marks exercised |
|---|---|
| **Spending by category** | `BarMark` (horizontal), `RuleMark` (budget target) |
| **Balance / net worth over time** | `LineMark` + `AreaMark` (v2) |
| **Budget vs. actual, by month** | grouped/stacked `BarMark` |
| **Category allocation** | `SectorMark` donut (v2) |
| **Spending calendar** | `RectMark` heatmap (day-of-year grid) |
| **Cash-flow range per month** | `RangeMark` (in/out) |

Why it's the right driver: broadest mark coverage, clean **local** data (CSV, no
server), directly on-thesis (replaces a ~$100/yr subscription), and pairs with the
existing `finance-cli`. It also stresses **`Time`/`Band` scales, dark-mode palettes,
selection tooltips, and animated re-layout on filter changes** — i.e. the whole lib.

### Named alternatives

- **Health / Activity dashboard** — the most Apple-Charts-thematic: weight `LineMark`,
  steps/sleep `BarMark`, blood-pressure `RangeMark`, activity `RectMark` heatmap.
  Slightly worse as a driver only because manual data entry is tedious (real value
  needs HealthKit import — mobile-gated).
- **Workout progress** — 1RM `LineMark`, weekly volume `BarMark`, training-day
  heatmap. Narrower mark variety.
- **Portfolio / stocks** (candlestick + area) — great charts, but live quotes need the
  web-networking gap; less local-first.

`docs/example-app-ideas.md` already lists budget, health, and workout under §A/§D;
this promotes **Ledger** to the concrete chart-library driver.

---

## Staging

Each stage ends runnable and golden-tested; mirrors the `games-plan` discipline.

- **C0 — Skeleton + measurement + one golden (small).** `chart` package: `Scale`
  (Linear + Band), `Mark` interface, `BarMark`, the `Chart` widget with
  painter-measured insets and axis rendering. Exit: a headless golden of a labelled
  bar chart, plus scale unit tests (`Map`/`Invert`/`Ticks` "nice" numbers).
- **C1 — The "today" library (medium).** `LineMark`, `PointMark`, `RuleMark`,
  `RectMark` (heatmap), `RangeMark`; `Time`/`Log`/`ColorScale`; axes (grid, tick
  formatting, rotated/elided labels), legend, dark-mode palettes, `Selection` +
  tooltip overlay, and data-change animation. Exit: a golden per mark type + an
  interaction test (tap → correct `Selection.Index` via `Scale.Invert`).
- **C2 — `paint.Path` marks (medium; shared with games-plan §WS3).** Land
  `paint.Path` + `FillPath`/`StrokePath` in `paint` (with the `gpu_equiv_test.go`
  CPU/GPU-parity guard), then `AreaMark`, `SectorMark`, and `LineMark.Smooth`. Exit:
  goldens for area/donut/smoothed + the parity test.
- **C3 — Ledger dashboard (medium).** The example app: CSV/OFX import, the six views
  above, filter chips, local-file persistence, desktop + web. Exit: a shippable
  dashboard whose every panel is a `chart` mark; render tests via `stateHook` +
  pixel probes.
- **Polish.** Accessibility `Semantics` overlay; `Ctx.DevicePixelRatio()` for hairline
  gridlines; export-to-PNG (already have headless render).

**Framework changes required:** only **`paint.Path`/`FillPath`/`StrokePath`** (C2),
which the games workstream wants anyway; everything in C0–C1 rides on primitives that
already exist. Optional: `Ctx.DevicePixelRatio()`.

## Scope guard (so `chart` doesn't grow a stats engine)

Package-doc gate, à la `game`: *"`chart` renders declarative marks over scales. It is
not a data-frame, statistics, or query library: it does no aggregation, regression,
binning, or CSV parsing — apps prepare `[]Datum` themselves. Marks and scales only.
If a proposed addition would be at home in pandas or a BI tool, it does not belong
here."* Aggregation helpers, if ever wanted, live in the example app, not the package.

## Testing

- Headless golden images per mark and per scale configuration (deterministic).
- Pure unit tests for scales (`Map`/`Invert` round-trip, `Ticks` niceness/count).
- The `app/gpu_equiv_test.go` guard gains a case for any new `paint` primitive
  (`FillPath`/`StrokePath`) — the standing rule that every Canvas op renders
  identically on CPU and GPU.
- Interaction tests drive `Interactive` taps and assert `Selection`.
