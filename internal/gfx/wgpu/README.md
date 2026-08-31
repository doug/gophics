# wgpu

> Vendored into gophics from the gogpu lineage; see `THIRD_PARTY.md`.
> This describes the package as it exists here, as an `internal/` dependency —
> it is not separately installable, and the upstream project's own docs may differ.

---

## Overview

**wgpu** is the unified Go WebGPU package with three independent implementations selected by build tags — like Chrome (Dawn) and Firefox (wgpu) implementing the same W3C spec.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Backends** | Vulkan, Metal, DirectX 12, OpenGL ES, Software, **Browser WebGPU**, **Rust FFI** |
| **Platforms** | Windows, Linux, macOS, iOS, **Browser (WASM)**, **Android/arm64 (preview)** |
| **API** | WebGPU-compliant (W3C specification) |
| **Shaders** | WGSL via gogpu/naga compiler (SPIR-V, HLSL, MSL, GLSL, DXIL) |
| **Compute** | Full compute shader support, GPU→CPU readback |
| **Present** | Damage-aware presentation — compositor dirty rects (first WebGPU implementation) |
| **Debug** | Leak detection, error scopes, validation layers, DRED diagnostics (DX12), structured logging (`log/slog`) |
| **Build** | Zero CGO, simple `go build` |

---

## Architecture

```
wgpu/
├── *.go                # Public API (import "github.com/gogpu/wgpu")
├── core/               # Validation, state tracking, deferred resource destruction
├── hal/                # Hardware Abstraction Layer
│   ├── allbackends/    # Platform-specific backend auto-registration
│   ├── noop/           # No-op backend (testing)
│   ├── software/       # CPU software rasterizer (~14K LOC)
│   ├── gles/           # OpenGL ES 3.0+ (~12K LOC)
│   ├── vulkan/         # Pure Go Vulkan backend (~42K LOC)
│   ├── metal/          # Metal (~7K LOC)
│   └── dx12/           # DirectX 12 (~17K LOC)
├── examples/
│   ├── compute-copy/   # GPU buffer copy with compute shader
│   └── compute-sum/    # Parallel reduction on GPU
└── cmd/
    ├── vk-gen/         # Vulkan bindings generator
    └── ...             # Backend integration tests
```

### Public API

The root package (`import "github.com/gogpu/wgpu"`) provides a safe, ergonomic API aligned with the W3C WebGPU specification. It wraps `core/` and `hal/` into user-friendly types:

```
User Application
  ↓ import "github.com/gogpu/wgpu"    ← always the same import
Root Package (public API: *Device, *Buffer, *Texture...)
  ↓ build tag selects implementation
  ├─ [default]      _native.go  → core/ → hal/ → vulkan/metal/dx12/gles/software
  ├─ [-tags rust]   _rust.go    → go-webgpu/webgpu → wgpu-native
  └─ [js,wasm]      _browser.go → syscall/js → Browser WebGPU
```

### Native HAL Backend Integration

For the default (Pure Go) path, backends auto-register via blank imports:

```go
import _ "github.com/gogpu/wgpu/hal/allbackends"

// Platform-specific backends auto-registered:
// - Windows: Vulkan, DX12, GLES, Software
// - Linux:   Vulkan, GLES, Software
// - macOS:   Metal, Vulkan, Software
// - Android/arm64 preview: Vulkan only
```

---

## Backend Details

### Platform Support

| Backend | Windows | Linux | macOS | iOS | Android/arm64 | Notes |
|---------|:-------:|:-----:|:-----:|:---:|:-------------:|-------|
| **Vulkan** | Yes | Yes | Yes | - | Preview | MoltenVK on macOS; [Android contract](docs/ANDROID.md) |
| **Metal** | - | - | Yes | Yes | - | Native Apple GPU |
| **DX12** | Yes | - | - | - | - | Windows 10+ |
| **GLES** | Yes | Yes | - | - | - | OpenGL ES 3.0+ |
| **Software** | Yes | Yes | Yes | Yes | - | CPU fallback |

**Architectures:** amd64, arm64 (including Windows ARM64 / Snapdragon X)

### Vulkan Backend

Pure Go Vulkan backend with:

- Auto-generated bindings from official `vk.xml`
- Buddy allocator for GPU memory (O(log n), minimal fragmentation)
- Dynamic rendering (VK_KHR_dynamic_rendering)
- Classic render pass fallback for Intel compatibility
- wgpu-style swapchain synchronization
- MSAA render pass with automatic resolve
- Complete resource management (Buffer, Texture, Pipeline, BindGroup)
- Surface creation: Win32, X11, Wayland, Metal (MoltenVK), and Android `ANativeWindow` (preview)
- Debug messenger for validation layer error capture (`VK_EXT_debug_utils`)
- Structured diagnostic logging via `log/slog`

### Metal Backend

Native Apple GPU access via:

- Pure Go Objective-C bridge (goffi)
- Metal API via runtime message dispatch
- CAMetalLayer integration for surface presentation
- MSL shader compilation via naga

### DirectX 12 Backend

Windows GPU access via:

- Pure Go COM bindings (syscall, no CGO)
- DXGI integration for swapchain and adapters
- Flip model with VRR support
- Descriptor heap management with fence-based deferred destruction
- Encoder pool with allocator recycling (Rust wgpu-core pattern)
- In-memory shader cache (SHA-256 keyed, LRU eviction, works for both paths)
- DRED diagnostics (auto-breadcrumbs + page fault tracking on TDR)
- **Dual shader compilation:** HLSL→FXC (default, SM 5.1) or **DXIL direct** via naga (`GOGPU_DX12_DXIL=1`, SM 6.0+, zero external dependencies — first Pure Go DXIL generator)
- StagingBelt ring-buffer allocator for zero-allocation GPU data transfer

### OpenGL ES Backend

Cross-platform GPU access via OpenGL ES 3.0+:

- Pure Go EGL/GL bindings (goffi)
- Full rendering pipeline: VAO, FBO, MSAA, blend, stencil, depth
- WGSL shader compilation (WGSL → GLSL via naga)
- Combined texture-sampler binding via SamplerBindMap (Rust wgpu pattern)
- Text rendering with proper texture completeness handling
- CopyTextureToBuffer readback for GPU → CPU data transfer
- Platform detection: X11, Wayland, Surfaceless (headless CI)
- Works with Mesa llvmpipe for software-only environments

### Software Backend

Full-featured CPU rasterizer for headless and windowed rendering. Always compiled — no build tags or GPU hardware required.

```go
// Software backend auto-registers via init().
// No explicit import needed when using hal/allbackends.
// For standalone usage:
import _ "github.com/gogpu/wgpu/hal/software"

// Use cases:
// - CI/CD testing without GPU
// - Server-side image generation
// - Reference implementation
// - Fallback when GPU unavailable
// - Embedded systems without GPU
```

**Rasterization Features:**
- Edge function triangle rasterization (Pineda algorithm)
- Perspective-correct interpolation
- Depth buffer (8 compare functions)
- Stencil buffer (8 operations)
- Blending (13 factors, 5 operations)
- 6-plane frustum clipping (Sutherland-Hodgman)
- 8x8 tile-based parallel rendering
- **SPIR-V interpreter** — executes vertex/fragment/compute shaders on CPU. Designed for shader debugging, CI/CD testing, and GPU-less environments — **not for production rendering** (interpreted, ~100× slower than JIT software renderers like SwiftShader).

**Debug & Testing:**
- Render pass instrumentation: `hal.Logger().Debug()` events + `RenderPassStats` for CI e2e assertions
- Public `wgpu.HeadlessSurfaceTarget` + `Surface.ReadPixels()` lifecycle for deterministic headless render verification; snapshots are owned, tightly packed RGBA8
- HAL `GetFramebuffer()` remains as a compatibility alias for existing software-backend callers; new root API code should use `Surface.ReadPixels()`
- Damage-aware partial blit with pixel-level test coverage

See [Surface targets](docs/SURFACE-TARGETS.md#headless-software-surface-and-readback)
for the complete configure → acquire → render → submit → present → readback
recipe and the explicit non-WebGPU support contract.

**Windowed Presentation:**
- **Windows:** DWM-safe `CreateDIBSection` + `BitBlt` (SDL3/Qt6 pattern), zero-copy into GDI bitmap
- **Linux X11:** `XPutImage` via goffi (Skia pattern), BGRA = X11 ZPixmap native format
- **macOS:** CGImage + `setContents:` (CALayer) or Metal `nextDrawable` + `replaceRegion` (CAMetalLayer). Contributor: @k-chimi

---

## Environment Variables

| Variable | Values | Description |
|----------|--------|-------------|
| `GOGPU_DX12_DXIL` | `1` | Enable DXIL direct compilation on DX12 (experimental). Bypasses HLSL→FXC, generates DXIL bytecode directly from naga IR. SM 6.0+, zero external dependencies. Default: off (uses HLSL→FXC). |
| `GOGPU_DX12_DXIL_OVERRIDE_VS` | file path | Replace vertex shader DXIL with contents of the given file. For debugging only. |
| `GOGPU_DX12_DXIL_OVERRIDE_PS` | file path | Replace pixel shader DXIL with contents of the given file. For debugging only. |

> **Note:** Backend selection (`GOGPU_GRAPHICS_API`) is handled by `gogpu` (the app framework), not by `wgpu` directly. See [gogpu documentation](https://github.com/gogpu/gogpu) for `GOGPU_GRAPHICS_API=vulkan|dx12|metal|gles|software`.

---

## References

- [WebGPU Specification](https://www.w3.org/TR/webgpu/) — W3C standard
- [wgpu (Rust)](https://github.com/gfx-rs/wgpu) — Reference implementation
- [Dawn (C++)](https://dawn.googlesource.com/dawn) — Google's implementation
- [Architecture Deep-Dive (Chinese)](https://chenxutan.com/d/1987.html) — Performance benchmarks, Snatchable pattern analysis, zero-alloc hot paths

---
