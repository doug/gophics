# The API surface gate

`testdata/api-surface.txt` is the committed public API of
`github.com/doug/gophics`, one line per element, with signatures.
`go test ./internal/apisurface` fails when the tree and the file disagree;
`GOPHICS_UPDATE_API=1 go test ./internal/apisurface` rewrites it so the change
shows up as a reviewable diff.

For a framework the compatibility guarantee *is* the product, so the point of
the file is not the number of lines in it. The point is that a reviewer can
read its diff and say "that is additive" or "that breaks people" without
opening any other file.

## Why signatures

The manifest used to record names and kinds and nothing else:

```
widget.Text.Wrap field
app.Run func
paint.Canvas.TextIn method
```

That is a gate against renames — which the compiler already finds, in this
repo, on the next build. It is not a gate against the way compatibility
actually gets broken. Change a parameter type, add a result, retype a struct
field, add a method to an interface: every one of those left the file
byte-identical, the build green, and every app outside this repo broken. The
caller who would have failed is not in the tree, so nothing here noticed.

There is a live example in the tree of the second failure mode, which is
subtler. `paint.Measurer` was added to the `paint.Canvas` contract — an
interface with implementations outside this module. The old format did record
it, because new methods are new *names*: four `+` lines arrived, reading
exactly like a new type and some new methods, which is to say reading as
additive. Nothing on those lines said that every outside implementation of
`Canvas` had just stopped compiling. Now the same change is three
`+ ... ifacemethod` lines on `paint.Canvas`, `Delta.Breaking` lists them as
breaking implementers, and the reviewer is told rather than trusted to know.

## Format

```
<package>.<Name>[.<Member>] <kind> <signature>
```

`kind` is one of `type alias field method ifacemethod func const var`.

The package path is module-relative, so the line reads the way the import
reads: `widget.Text.Wrap`, `shell/desktop.Run`, `sound/mp3.Decode`.

Examples, verbatim from the file:

```
anim.Controller.Tick method func(*anim.Controller, float64) bool
app.Run func func(widget.Widget, app.Config) error
geom.Pt type struct
geom.Pt.X field float32
paint.Canvas type interface
paint.Canvas.TextIn ifacemethod func(string, string, geom.Pt, float32, paint.Color)
shell.CursorDefault const shell.CursorShape = 0
widget.Text.Wrap field bool
```

### Reading the diff

The name and the kind are the key, the file is sorted by it, and neither
contains a space. So:

| in the diff | what it means |
| --- | --- |
| a lone `+` | additive — a v1 package may do this |
| a lone `-` | a removal: a break |
| a `-`/`+` pair on the same key | the signature moved: a break |
| a `+` on an `ifacemethod` | an interface grew a method: breaks implementers |

Sorting by the key is what makes the third row work: the two halves of a
signature change land adjacent, as one hunk, instead of as an unrelated
removal and addition hundreds of lines apart. `apisurface.Diff` applies the
same rule in code, and `Delta.Breaking` is the list the failure message leads
with.

`ifacemethod` exists as a kind of its own because of the fourth row. An added
interface method is a single `+` line and looks exactly as additive as a new
struct field, and it is the opposite: every implementation outside this repo
stops compiling. The kind is what lets the tooling tell those two apart.

### Conventions, and why each one

**Parameter names are omitted.** Go has no keyword arguments, so renaming a
parameter cannot break a caller. If names were recorded, a rename would appear
as a `-`/`+` pair on a stable key — indistinguishable from a type change,
which is the one thing this file exists to make obvious. A gate that reports
harmless churn as a break gets switched off.

**A method carries its receiver as its first parameter** — the method
expression form, `Controller.Tick` rather than `(c *Controller) Tick`. Moving a
method from a value receiver to a pointer receiver drops it from the value's
method set, so a value that satisfied an interface stops satisfying it. That is
a break, and it is invisible in every other part of the line.

**An interface records its whole method set,** embedded methods included
(`types.Interface.NumMethods`, not `NumExplicitMethods`). An implementer has to
provide them all; which side of an embed they were written on is this module's
business, not theirs.

**A constant records its type and value.** For `theme.FontBold` the value *is*
the API — apps write the string literal as a map key. For an `iota` block the
value is whatever has already been persisted or sent on a wire, so renumbering
one is a data break even when no Go code changes.

**A defined type records its underlying type:** `widget.Direction type uint8`.
Retargeting an alias or changing an underlying type breaks conversions and
untyped constant assignment, and the member lines underneath would not move.

**Floating-point constants print as the shortest `float64` rendering** —
`0.5`, not the `1/2` that `constant.Value.ExactString` produces. The short form
round-trips, so it still separates any two values a Go program can hold, and
it is the form the source says.

**Members of a variable or constant are not expanded.** The old format listed
`geom.Unbounded.Min`, `intl.Default.Decimal`, `shell.CursorDefault.String` —
the whole member list of a type, repeated under every exported value of that
type. The new format records the value's type instead (`geom.Unbounded var
geom.Rect`) and the members once, under the type. Nothing is lost: the pair of
lines pins down everything the expansion did, and it does so without 87 lines
of duplication.

## What the gate still cannot see

Worth knowing, so nobody reads a green run as more than it is.

- **Behaviour.** A function that keeps its signature and changes what it
  returns is a break the manifest cannot have an opinion about.
- **Unexported struct fields.** Adding one breaks unkeyed composite literals
  (`geom.Pt{1, 2}`) and can break comparability. The manifest records exported
  fields only, so that change is silent here.
- **Struct tags.** Not Go API; they are a serialization contract, and if one
  ever matters it belongs in that subsystem's own gate.
- **Parameter names,** deliberately — see above. A documented name that changes
  is a doc change.
- **Per-platform divergence.** The surface is the *union* over the six targets
  in `Targets`. A name that exists on one platform and not another is one line,
  not two. There are no keys today whose signature differs between targets; if
  one appears, the union silently keeps whichever target was scanned last, and
  that is the point at which the format needs a platform column.

## Stability tiers

The table is in `tiers.go`, in Go, because the tests read it. It is generated
into the manifest header so the two cannot drift.

The module is pre-1.0 and the README says plainly that renames land eagerly
while the cost is low. So a tier here is a **proposal**: the list of packages
whose shape is believed settled enough to freeze at v1, and the list that is
deliberately not. Writing it down costs nothing before the tag and is the only
way "a small surface and the Go 1 compatibility ethos after 1.0" becomes a
decision rather than a feeling. It also answers the question somebody
evaluating the toolkit actually has, which is not how large the surface is but
which of their imports will survive the tag.

### Proposed v1-committed

`anim app geom input intl layout paint shell shell/desktop shell/terminal
shell/web text theme widget`

These are the imports in every gophics program. They have the most use behind
them, and their shape is what the project is opinionated about.

- `geom` — value types and pure arithmetic. Nothing about `Pt`, `Rect` or
  `Insets` is waiting on a decision.
- `paint` — `Color`, `Path`, `Transform` and `Canvas`. Committed **for
  callers**. `Canvas` is an interface, and adding a method to it breaks anyone
  who implements one; that happened this month, with `Measurer`. If outside
  implementations are a thing gophics wants to support, `Canvas` needs the
  usual escape hatch — an unexported method, or a `BaseCanvas` to embed —
  before v1, and that is a decision this file can only point at.
- `layout` — the `RenderObject` protocol. Narrower than the rest, but a custom
  render object implements it, and a custom render object is the documented way
  to do something the widget catalog does not cover. If it is not committed,
  that door is not really open.
- `widget`, `theme`, `anim`, `input`, `text` — the tree, the token set, and the
  types that appear in their signatures. `text` is here mostly by implication:
  its types show up in `paint` and `widget` signatures, so it is already
  committed whatever the table says.
- `shell` — the capability interfaces an app reaches through `ctx`. The biggest
  package in the surface by a wide margin, and committable only under one
  discipline: capabilities grow by **adding interfaces**, never by adding a
  method to an interface that exists. `nil` is the answer for a capability a
  platform does not have, so a new capability is a new interface and an old one
  never needs a new method. The `ifacemethod` kind is in the manifest so that
  rule is enforceable rather than remembered.
- `shell/desktop`, `shell/terminal`, `shell/web` — three entry points called
  once each from a `main`. Config in, error out. There is nothing left there to
  settle.
- `intl` — **not by plan.** `widget.Ctx.IntlLocale` returns `intl.Locale`, so
  the type is already load-bearing in a committed signature and the standing
  plan to internalize the package is incompatible with that. The leak check
  found this; it is the sort of thing the tier table is for.

### Explicitly unstable

`apptest chart fetch sound sound/device sound/mp3 sound/ogg sound/pitch
sound/procedural tools/uitrace/trace`

Importing these is a choice to track the repo. They break in any v0.x, and
after v1 they break in a minor release with a note.

- `apptest` — the golden-image tolerance model is still moving; `Tol` and
  `AntiAliased` arrived to fix cross-machine failures and are not obviously the
  final shape. A break here is a compile error in a test, which is the cheapest
  kind of break there is: it surfaces on the next `go test` rather than in
  somebody's shipped app.
- `chart` — a catalog that compounds. Marks arrive, get renamed, and grow
  options. Freezing it would freeze the wrong thing early.
- `fetch` — exists to keep `net/http` out of a wasm binary, which is a linker
  problem, not a design. If the toolchain fixes it the package's reason to
  exist goes away, and a package that might be deleted cannot be committed.
- `sound/**` — the least exercised layer in the tree.
- `tools/uitrace/trace` — a trace file format, versioned with its viewer rather
  than with the module.

### What the tiers buy, enforced

Two tests, both of which can only work now that signatures are recorded:

1. **Every public package has a tier.** A new public package fails the gate
   until somebody decides what it promises — which is the only moment the
   decision is cheap. After it has callers, the decision has been made for you.
2. **No v1 signature names something a v1 caller cannot commit to**, meaning a
   type from an unstable package or from `internal/`. A committed name that
   needs an uncommitted type is a promise with a hole in it.

Known holes live in `tierExceptions` in `tiers.go`, each with its reason, and
`StaleExceptions` deletes-by-failing any that stop describing anything. Both
current entries were found by this check on its first run:

- `app.Headless.Scene` returns `*internal/scene.List`. Nobody outside the
  module can name the type, so the method is a token-passing affordance for
  tests. It wants an exported projection or it wants to leave v1.
- `paint.Painter.GPUCanvas` takes `*internal/gfx/gg.Context`, a handle from the
  vendored substrate. Reaching the GPU canvas is a real need; the parameter
  should be a `gpucontext` token — the shape the shells already use to pass a
  device across package boundaries — rather than a vendored type.
