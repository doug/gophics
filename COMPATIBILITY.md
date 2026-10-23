# Compatibility

What gophics promises about its API, which packages the promise covers, and how
the promise is enforced.

**Status: proposed, not yet in force.** The module is tagged `v0.1.0` and
nothing below binds until a `v1.0.0` tag exists. This file is the shape of the
commitment, written down so it can be argued with before it is made.

## The promise

Within `v1.x`, the packages listed as covered change **additively only**:

- No exported name is removed or renamed.
- No function, method, or field changes its type.
- No method is added to an exported interface.
- No exported struct used as a value gains or loses a field in a way that
  breaks a positional composite literal.

New packages, new functions, new methods on concrete types, and new fields on
structs documented as "constructed by name" are additive and may land in any
minor release.

Breaking changes go in `v2`, under a new module path, with the old major kept
buildable. There is no deprecation-and-removal path inside `v1`.

### Why "no method added to an exported interface" is on that list

It is the one entry people are surprised by, and it is the one this repo has
already been bitten by. `paint.Canvas` is implemented outside this module — a
backend is the whole point of it — so adding a method to it breaks every
implementer, while reading in a diff exactly like new API. `paint.Measurer`
went in that way before the manifest recorded signatures.

This is why several things in `paint` are *opt-in capabilities* (`Blender`,
`GradientFiller`) rather than methods on `Canvas`: a capability an
implementation has not heard of costs it nothing, and `nil` is a real answer.
Expect that pattern to be how drawing grows during `v1`.

## Covered packages

Fourteen. These are the imports in an ordinary gophics program.

| Package | Why it is settled |
| --- | --- |
| `anim` | curves and a controller; the signatures are numbers |
| `app` | `app.Run` and `app.Headless` are the entry points |
| `geom` | value types and pure math, no churn pressure |
| `input` | event types; a new event kind is additive |
| `layout` | the `RenderObject` protocol a custom widget implements |
| `paint` | `Color`, `Path`, `Canvas` — see the interface note above |
| `shell` | the capability interfaces an app calls through `ctx` |
| `text` | shaping types that appear in `paint` and `widget` signatures |
| `theme` | design tokens and styled components |
| `widget` | the widget tree itself |
| `intl` | not by plan: `widget.Ctx.IntlLocale` returns `intl.Locale` |
| `shell/desktop` | three exported names between them, each called once from |
| `shell/terminal` | a `main` package: a window config in, an error out |
| `shell/web` | |

## Explicitly not covered

These may break in any release, including a `v1` patch. Each is either young,
optional, or tooling, and a break in one is cheap to absorb — a compile error
in a test, or a subsystem you opted into.

`apptest`, `chart`, `fetch`, `sound`, `sound/device`, `sound/mp3`,
`sound/ogg`, `sound/pitch`, `sound/procedural`, `tools/uitrace/trace`

`apptest` is the one worth calling out: it is the test harness, so an app's
*tests* can break at a minor release even when the app cannot. That is
deliberate — the golden-image tolerance model is still moving, and freezing it
early would be a worse trade than making you edit a test.

Anything under `internal/` is not API at all. That includes the vendored
rendering substrate (`internal/gfx/...`), which carries its own upstream
licenses and is not a surface this project can promise.

## Named exclusions

One member of a covered package is outside the promise:

- **`paint.Painter.GPUCanvas`** takes `*internal/gfx/gg.Context`, a handle from
  the vendored substrate. No caller outside this module can obtain one, so the
  method is already unreachable from outside and excluding it costs nobody
  anything. Closing it properly needs an exported substrate handle, which is a
  decision about the fork boundary rather than an API cleanup.

An exclusion is not a loophole: the list is in `internal/apisurface/tiers.go`
as `tierExceptions`, every entry carries its reason, and the gate fails on a
*new* one. An empty map is the goal.

## How this is enforced

Not by intent. `internal/apisurface/testdata/api-surface.txt` is the committed
public API, one line per element **with signatures**, and
`go test ./internal/apisurface` fails when the tree and the file disagree.
Regenerate it with `GOPHICS_UPDATE_API=1 go test ./internal/apisurface` so the
change arrives as a reviewable diff.

Signatures rather than names is the part that matters. A name-only manifest is
a gate against renames, which the compiler already catches on the next build.
Retyping a parameter, adding a result, or adding a method to an interface left
the old file byte-identical and every outside app broken.

Two further checks run off the same manifest:

- **`Delta.Breaking`** classifies a diff, so a reviewer is told that a change
  breaks implementers rather than trusted to notice.
- **`Leaks`** reports covered entries whose signature names a type a caller
  cannot commit to — something from `internal/`, which is unnameable, or from an
  uncovered package, which is a promise with a hole in it. Every package must be
  classified before the gate passes.

`internal/apisurface/README.md` has the per-package reasoning and the format.

## What is not promised

- **Behaviour is not frozen, only the API.** Layout, rasterization, and
  shaping output improve during `v1`; golden images are expected to need
  re-approval.
- **Platform coverage is not frozen.** A capability that returns `nil` on a
  platform today may return an implementation tomorrow. That is additive by
  construction — your UI already handles `nil`, because `nil` is the documented
  answer.
- **The web payload, build times, and performance** are not covered by any
  compatibility statement.
