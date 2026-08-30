# Which packages promise what

PLAN.md §1.7 commits to "a small surface and the Go 1 compatibility ethos after
1.0". That is one promise made to packages with very different jobs: an app
names `widget.Text`, a backend implements `shell.Window`, and freezing both at
the same moment would either hold the platform layer back or break apps.

So the promise is per tier. The tiers below are what each package is *for*, and
the guarantee follows from that.

Counts come from `design/api-surface.txt`, which is generated and enforced —
`internal/apisurface` fails a test when the tree and the file disagree, so the
numbers in this table cannot quietly drift from the code.

| Tier | Packages | Names | After 1.0 |
|---|---|---:|---|
| **1 — App** | `app` `widget` `theme` `chart` `paint` `geom` `anim` `intl` `apptest` `sound`+4 | 1,302 | **Frozen.** Additive only, no signature changes, no removals. |
| **2 — Extension** | `layout` `text` `input` | 200 | **Stable.** For custom widgets and embedding hosts. Breaks only with a major version and a migration note. |
| **3 — Platform** | `shell` `shell/{desktop,terminal,web}` | 454 | **Additive only.** A backend may gain capabilities; existing ones do not change shape. |
| — Excluded | `shell/mobile` | — | Versioned with the host app, not with this module. |

## Tier 1 — what an app writes

This is the tier the promise is really about. An app that compiles against 1.0
should compile against every 1.x.

`geom` at 53 names is the model the rest could follow: four value types,
consistent `Lerp` methods, one sentinel, nothing speculative. `anim` and `input`
are similarly finished.

`theme` and `chart` are large and honestly so. They are component and mark
libraries, where most of the count is struct fields that *are* the configuration
API — a `Button` with eight fields is eight names and one concept. The same
covers `widget`'s fields: the declarative style trades exported fields for
constructor functions on purpose, and the trade is the reason the widget code an
app writes reads as data.

## Tier 2 — what a custom widget or an embedder needs

`layout` is the render protocol: implement `Box`, take `Constraints`, return a
size, answer `AddHits` and `Semantics`. It used to be 372 names, because the
sixteen concrete boxes lived here beside the protocol describing them and
shadowed the widget layer name for name. They are in `internal/layoutbox` now.
Across every example in the module, this package is used for eleven names, all
of them protocol.

The tier is stable rather than frozen because a genuinely new layout capability
may need the protocol to change, and pretending otherwise would push the change
into a worse shape somewhere else.

## Tier 3 — what a backend implements

`shell` is the contract between the runtime and a platform: `Window`, `Handler`,
`Frame`, `Target`, `PixelTarget`, the event types, and the capability
interfaces. Additive-only after the cleanup, which is what makes an out-of-tree
backend viable — `examples/embed-ebiten` is one, and it is the test of whether
the promise is worth anything.

Two things this tier deliberately does not offer.

**GPU presentation is in-module.** There was a portable `GPUTarget` carrying a
WebGPU texture view and nothing ever constructed one. It is not re-addable
either: two Go WebGPU bindings cannot exchange a `Device` through Go types, so a
host cannot hand gophics a device from its own binding. This is not a visibility
problem, and publishing the vendored substrate would not fix it — it would
export `ggcanvas`, `gg.Context` and several hundred names to work around a
limitation that would still be there. An embedding host presents through
`PixelTarget`, and the damage rect is what makes that affordable.

**Capabilities are opt-in interfaces, not a registry.** A backend implements
`shell.BatteryWindow` or it does not, and `ctx.Battery()` is nil where it does
not. That nil is the API: it is how an app knows to hide an affordance rather
than show a button that does nothing.

## `shell/mobile` is excluded, not exempt

Its `Bridge` carries ~130 exported methods and every one is called from Kotlin
or Swift. Exporting them is what gomobile requires; freezing them is not
something this module can usefully promise, because they are versioned with the
host app that binds them.

A module split would fight the build rather than help it: `gomobile bind` binds
`Bridge` in the same invocation as the app's own package, it needs
`golang.org/x/mobile` in the module, and `design/substrate-consolidation.md`
records that consolidating *into* one module is what fixed the
gomobile-ignores-`go.work` trap. So it is a documented exclusion and an
exclusion from the generated manifest, which is why the total above is
1,956 rather than well over 2,000.

## How this is kept honest

`internal/apisurface` writes `design/api-surface.txt` and a test fails on drift,
listing what was added and removed. Regenerate deliberately:

    GOPHICS_UPDATE_API=1 go test ./internal/apisurface

The manifest is the union across six GOOS/GOARCH targets, because the shells are
build-constrained: a scan on one platform cannot see `shell/web.Run` at all, and
the first version of this tool omitted a whole public package while reading as
complete.

It also scans the separate modules. The audit that preceded this work concluded
"nothing imports `intl`" and was wrong — `examples/tally` does, and calls
`intl.Auto()` on Android and iOS, where the environment variables it reads do
not exist. It was missed because tally is its own module and invisible to
`go list ./...` from the root. `apisurface.Consumers` exists so the next "no
caller anywhere" claim has to survive that.
