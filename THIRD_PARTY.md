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

These packages are `internal/` on purpose: they are gophics's private
implementation substrate, not a public API. See
`design/substrate-consolidation.md` for the rationale and structure.

## Still-external dependencies

The bottom of the GPU stack remains an ordinary module dependency (not vendored):
`github.com/go-webgpu/{goffi,webgpu}` (the FFI/dlopen floor). Text shaping
(`github.com/go-text/typesetting`), audio decoders
(`github.com/hajimehoshi/go-mp3`, `github.com/jfreymuth/oggvorbis`), and the
`golang.org/x/*` packages are also external. None require CGo; the whole module
builds with `CGO_ENABLED=0` (the mobile `gomobile bind` path in `shell/mobile`
is the one contained exception).
