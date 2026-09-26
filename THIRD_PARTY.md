# Third-party code

Gophics vendors its GPU/graphics/audio substrate directly into the module (under
`internal/gfx/` and `internal/audio/`) rather than depending on separate
repositories. These are maintained forks of the `github.com/gogpu/*` lineage,
all MIT-licensed. Each vendored tree keeps its original `LICENSE` file; the
copyright notices there are retained as MIT requires.

| Vendored path | Origin | License |
|---|---|---|
| `internal/gfx/gg` | `github.com/gogpu/gg` (via `github.com/doug/gg`) — 2D vector renderer | MIT |
| `internal/gfx/wgpu` | `github.com/gogpu/wgpu` — pure-Go WebGPU implementation | MIT |
| `internal/gfx/naga` | `github.com/gogpu/naga` — WGSL→SPIR-V/MSL shader translator | MIT |
| `internal/gfx/gogpu` | `github.com/gogpu/gogpu` — windowing + higher-level GPU | MIT |
| `internal/gfx/gpucontext` | `github.com/gogpu/gpucontext` — opaque GPU handles | MIT |
| `internal/gfx/gputypes` | `github.com/gogpu/gputypes` — shared GPU types | MIT |
| `internal/audio` | `github.com/gogpu/audio` (via `github.com/doug/audio`) — audio output drivers | MIT |

## Local modifications

The vendored trees are forks and carry local changes beyond their upstreams.
`internal/gfx/gg` in particular has performance work not present in
`gogpu/gg`: a pooled free list for popped layer pixmaps in `context_layer.go`
and `pixmap.go`, and a row-copy fast path for identity (unscaled, untranslated)
image draws in `internal/image/draw.go`. It also carries a lifetime fix in
`internal/gpu/gpu_shared.go`: `GPUShared.Close` closes and drops the pooled
child render contexts (`childCtxPool`) before destroying the pipelines, and
`SetDeviceProvider` flushes that pool on any device change rather than only
when it still holds a differing device — the mobile shell closes the
accelerator and rebuilds the device on every rotation, and a pooled context
bound to the released device rendered its layer group into nothing. Each is
covered by tests in its own package; the licences and copyright notices are
unchanged.

These packages are `internal/` on purpose: they are gophics's private
implementation substrate, not a public API. The rationale is that gomobile ignores go.work, so a multi-module layout
could not be bound for Android or iOS at all.

## Still-external dependencies

The bottom of the GPU stack remains an ordinary module dependency (not vendored):
`github.com/go-webgpu/{goffi,webgpu}` (the FFI/dlopen floor). Text shaping
(`github.com/go-text/typesetting`), audio decoders
(`github.com/hajimehoshi/go-mp3`, `github.com/jfreymuth/oggvorbis`), and the
`golang.org/x/*` packages are also external. None require CGo; the whole module
builds with `CGO_ENABLED=0` (the mobile `gomobile bind` path in `shell/mobile`
is the one contained exception).
