# gputypes

WebGPU type definitions for the [gogpu](https://github.com/gogpu) ecosystem.

## Overview

`gputypes` provides all WebGPU enums, structs, and constants as pure Go types with **zero dependencies**. It serves as the **single source of truth** for WebGPU types across the entire ecosystem.

## Installation

```bash
go get github.com/gogpu/gputypes
```

**Requires:** Go 1.25+

## Why gputypes?

### The Problem: Type Incompatibility

Without a shared types package, each project defines its own types:

```go
// Different projects, incompatible types!
wgpu.TextureFormat        // wgpu's type
webgpu.TextureFormat      // go-webgpu's type
born.TextureFormat        // born-ml's type

// User must convert everywhere - painful!
format := wgpu.TextureFormat(webgpuFormat)
```

### The Solution: Shared Types

With `gputypes`, all projects use the same types:

```go
import "github.com/gogpu/gputypes"

// All projects use gputypes.TextureFormat
// Types are directly compatible - no conversion needed!
```

## Architecture

```
                    gputypes (ZERO deps)
                 All WebGPU types (100+)
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
    gpucontext          wgpu          go-webgpu/webgpu
  (imports gputypes)  (imports)         (imports)
         │                 │                 │
         └────────┬────────┴────────┬────────┘
                  │                 │
                  ▼                 ▼
               gogpu             born-ml
```

## Types

Based on [WebGPU spec](https://www.w3.org/TR/webgpu/) and [wgpu-types](https://docs.rs/wgpu-types):

### Texture & Sampler
- `TextureFormat` (97 formats including BC, ETC2, ASTC compressed)
- `TextureUsage`, `TextureDimension`, `TextureViewDimension`, `TextureAspect`
- `TextureDescriptor`, `TextureViewDescriptor`, `TextureSampleType`
- `AddressMode`, `FilterMode`, `MipmapFilterMode`, `CompareFunction`
- `SamplerDescriptor`, `SamplerBindingType`

### Buffer & Binding
- `BufferUsage`, `BufferBindingType`, `BufferMapState`, `MapMode`
- `BufferDescriptor`, `IndexFormat`
- `BindGroupLayoutEntry`, `BindGroupEntry`, `BindingResource`
- `BufferBindingLayout`, `SamplerBindingLayout`, `TextureBindingLayout`
- `StorageTextureBindingLayout`, `PipelineLayoutDescriptor`

### Shader
- `ShaderStage` flags (Vertex, Fragment, Compute)
- `ShaderModuleDescriptor`, `ShaderSource` (WGSL, SPIR-V, GLSL)
- `ProgrammableStage`

### Pipeline
- `PrimitiveTopology`, `FrontFace`, `CullMode`, `PrimitiveState` — zero value of each enum is the WebGPU spec default (`TriangleList`, `CCW`, `None`), so `PrimitiveState{}` is a valid spec-default configuration
- `BlendState`, `BlendFactor`, `BlendOperation`, `BlendComponent`
- `DepthStencilState`, `StencilOperation`, `StencilFaceState`
- `MultisampleState`, `ColorTargetState`, `ColorWriteMask`

### Vertex
- `VertexFormat` (31 formats)
- `VertexStepMode`, `VertexAttribute`, `VertexBufferLayout`
- `VertexState`, `FragmentState`

### Render Pass
- `LoadOp`, `StoreOp`
- `RenderPassColorAttachment`, `RenderPassDepthStencilAttachment`
- `RenderPassDescriptor`

### Adapter & Device
- `DeviceType`, `Backend`, `Backends`
- `AdapterInfo`, `PowerPreference`, `MemoryHints`
- `DeviceDescriptor`, `RequestAdapterOptions`
- `InstanceDescriptor`, `InstanceFlags`
- `Dx12ShaderCompiler`, `GLBackend`

### Limits & Features
- `Limits` struct with all WebGPU limits (30+ fields)
- `Features` flags (20 optional capabilities)
- `DefaultLimits()`, `DownlevelLimits()` helpers

### Surface
- `PresentMode` (AutoVsync, Fifo, Immediate, Mailbox)
- `CompositeAlphaMode` (Auto, Opaque, PreMultiplied, etc.)
- `SurfaceConfiguration`, `SurfaceCapabilities`, `SurfaceStatus`

### Copy Operations
- `ImageSubresourceRange` (for partial texture operations)
- `TextureDataLayout`, `ImageCopyTexture`

### Geometry & Color
- `Extent3D`, `Origin3D`
- `Color` (RGBA float64) with predefined colors

## Relationship to gpucontext

| Package | Purpose | Dependencies |
|---------|---------|--------------|
| `gputypes` | Data types (enums, structs, constants) | **ZERO** |
| `gpucontext` | Interfaces (DeviceProvider, EventSource, Texture) | imports gputypes |

### Why Two Packages?

| Aspect | gputypes | gpucontext |
|--------|----------|------------|
| **Responsibility** | Data definitions | Behavioral contracts |
| **Change frequency** | Rare (WebGPU spec is stable) | Medium (API evolution) |
| **Size** | Large (100+ types) | Small (10-15 interfaces) |

**Principle:** Separation of data types from behavioral interfaces.

### Why gpucontext imports gputypes?

Interfaces need types in their signatures:

```go
// gpucontext needs gputypes for type-safe interfaces
type DeviceProvider interface {
    SurfaceFormat() gputypes.TextureFormat  // ← uses gputypes
}

type Texture interface {
    Format() gputypes.TextureFormat  // ← uses gputypes
}
```

This ensures **type compatibility** across all implementations.

## Ecosystem

| Package | Description | Uses gputypes |
|---------|-------------|---------------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | Graphics framework | ✅ |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | ✅ |
| [gogpu/gg](https://github.com/gogpu/gg) | 2D graphics library | ✅ (via gpucontext) |
| [gogpu/gpucontext](https://github.com/gogpu/gpucontext) | Shared interfaces | ✅ (imports) |
| [go-webgpu/webgpu](https://github.com/go-webgpu/webgpu) | Rust FFI bindings | ✅ |
| [born-ml/born](https://github.com/born-ml/born) | ML framework | ✅ |

## Comparison with Rust

| Rust | Go (gogpu) | Purpose |
|------|------------|---------|
| `wgpu-types` | `gputypes` | WebGPU type definitions |
| — | `gpucontext` | Integration interfaces (Go-specific) |
| `wgpu-core` | `wgpu/core` | WebGPU implementation |
| `wgpu-hal` | `wgpu/hal` | Hardware abstraction |

## Status

See [CHANGELOG.md](CHANGELOG.md) for version history and [ROADMAP.md](ROADMAP.md) for upcoming plans.

## License

MIT License — see [LICENSE](LICENSE) for details.

---

*Part of the [gogpu](https://github.com/gogpu) ecosystem — Pure Go GPU Computing.*
