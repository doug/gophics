# Working on gophics

Guidance for coding agents changing this repository. If you are *using* gophics
to build an app, you want `skills/gophics/SKILL.md` instead — that teaches the
widget API. This file is about the codebase itself.

## Invariants

**Zero CGo.** `import "C"` anywhere fails CI. Everything reaches the platform
through syscalls, `purego`-style dynamic loading, or JS interop. This is the
constraint the whole project is organised around, so a fix that adds cgo is not
a fix.

**Capabilities are opt-in interfaces, and `nil` is a real answer.** A shell
implements `shell.BatteryWindow` or it does not, and `ctx.Battery() == nil`
means "this platform cannot tell you" — which an app uses to hide an
affordance. Never publish a capability whose implementation always returns
nothing: a caller cannot then distinguish "no controller connected" from "this
build cannot see controllers", and it will show a pairing prompt forever. If
you cannot implement it, leave it nil. `shell/coverage_test.go` will make you
record the reason.

**The public API is measured.** `internal/apisurface` generates
`internal/apisurface/testdata/api-surface.txt` and fails when the tree drifts
from it. Adding a name is fine; regenerate with
`GOPHICS_UPDATE_API=1 go test ./internal/apisurface` and let the diff be
reviewed. Note the manifest records names and kinds but *not* signatures, so it
will not catch a signature-only break.

## Before you push

```sh
./scripts/gates.sh        # gofmt, vet, generated files, tracked-file sizes
./scripts/ci-local.sh     # the CI jobs, locally: Linux in podman, macOS native
```

`gates.sh` also runs from `.githooks/pre-push`. **Read its last line, not its
last step** — `./scripts/gates.sh | tail -1` prints the name of the final check
and looks identical whether it passed or failed. That mistake shipped a gofmt
failure to a push and hid a real test failure in the same session.

`ci-local.sh` exists because `go test ./...` on a Mac does not exercise the
configurations CI does. It reads the commands out of the workflow file rather
than restating them, so it cannot drift. It does not catch
architecture-specific differences — see the script's header for exactly what it
misses.

## Build tags change what compiles

`go test ./...` is not full coverage:

| Tag | What it covers |
| --- | --- |
| *(none)* | the default path |
| `nogpu` | pure-CPU build; the GPU substrate compiles out entirely |
| `gophics_gpu` | GPU tests — equivalence, blur, readback; self-skip with no adapter |

A test file that references GPU symbols needs `//go:build !nogpu`, or the
`nogpu` job cannot compile. That is a real failure this repo has shipped.

## Testing on real devices

Zero CGo means test binaries cross-compile and run directly — no app, no JNI,
no gomobile:

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -tags gophics_gpu -o app.test.android ./app
adb push app.test.android /data/local/tmp/ && adb shell chmod +x /data/local/tmp/app.test.android
adb shell "cd /data/local/tmp && ./app.test.android -test.run TestX -test.v"
```

The same works for Windows over SSH. This is the cheapest way to get a second
backend's opinion, and it has repeatedly disagreed with the Mac's.

## Traps that have actually bitten

- **Logical pixels above, physical below.** `geom` and everything in the widget
  tree are logical; the painter's surface, damage rects handed to a presenter,
  and anything touching a texture are physical — logical × device scale. Mixing
  them is invisible at 1× and wrong at 2×, where a logical rect names the
  top-left quadrant of a physical surface. That shipped: the web presenter
  uploaded half the height of every frame and left the rest showing the previous
  frame. `app.present` is where the conversion belongs.
- **GPU handles cross package boundaries as `gpucontext` struct tokens**, not
  `interface{}` or raw pointers — `wgpu.DeviceFromHandle` turns one back into a
  device. It is what lets the layers pass a device around without importing each
  other.
- **Golden images across machines** need `apptest.Tol(apptest.AntiAliased)`.
  `Exact` is right within one machine and fails across two on sub-LSB float
  rounding — 4 pixels differing by 1/255 was enough.
- **`git add -A` sweeps build outputs.** Stage named paths. Compiled examples
  and measurement harnesses land in the repo root and have reached commits at
  19–27MB more than once; the size gate catches them, but only after the fact.
  They arrive from ordinary commands, not careless ones: `go build ./examples/x`
  writes `x` to the working directory, and `go test -cpuprofile` *keeps* the
  `pkg.test` binary after the run. Build with `-o /dev/null` when you only want
  to know it compiles, and delete the `.test` file after profiling. `.gitignore`
  now excludes extensionless files at the repo root by shape rather than by
  name, so a new example is covered without being listed.
- **Do not hand-edit generated blocks.** The capability matrix inside
  `internal/capgen/README.md` sits between `<!-- planfacts -->` markers and is
  rewritten by `scripts/tools/planfacts.py`; the prose around it is yours. The
  API manifest comes from `internal/apisurface`. `gates.sh` fails when either
  drifts.
- **`internal/gfx/` is vendored** from the MIT-licensed gogpu lineage. Its
  comments cite `ADR-NNN` documents that live upstream and are not readable
  here. Leave them: rewriting diverges the forks.
- **A test that skips is not a test that passes.** Several here are conditional
  (`if mismatches > 0 { t.Skipf(...) }`) and a few guard on a capability rather
  than a defect. When you add one, say which of the two it is in the skip
  message.

## Where things live

- `app/` `widget/` `layout/` `paint/` `geom/` — the framework
- `shell/{desktop,web,mobile,terminal}/` — platform backends
- `internal/gfx/` — vendored GPU/graphics substrate, see `THIRD_PARTY.md`
- `examples/` — some are separate modules with their own `go.mod`
- `skills/` — agent skills for *using* gophics
- Planning and design documents are **not in this repo**; they live in a
  separate private tree. Do not add a `design/` directory back.

## Comments

Explain *why*, and prefer putting the reason in the code over pointing at a
document. A comment that says "see design/x.md" is worthless the moment that
file moves, and 39 of them had to be rewritten inline when the planning docs
were split out. If a decision needs a page, it goes in the package doc comment
where `go doc` shows it — see `geom` for the shape.
