# Changelog

All notable changes to gputypes will be documented in this file.

## [v0.5.1] - 2026-06-28

### Fixed

- **`TextureFormat.BlockCopySize() uint32`** — canonical bytes-per-texel-block method on TextureFormat. Eliminates 3 independent buggy implementations across wgpu, gogpu, and gg (each had different wrong values — e.g., RG16 returned 8 instead of 4 in gogpu). Covers all 87 defined formats, verified against Rust wgpu-types `block_copy_size`. Returns 0 for implementation-defined formats (Depth24Plus). Table-driven tests with completeness guard.

## [v0.5.0] - 2026-04-21

### Changed (BREAKING)

- **PrimitiveTopology**: zero value is now `PrimitiveTopologyTriangleList` (was `PrimitiveTopologyUndefined`). Enum values renumbered: TriangleList=0, PointList=1, LineList=2, LineStrip=3, TriangleStrip=4. Removed `PrimitiveTopologyUndefined`.
- **FrontFace**: zero value is now `FrontFaceCCW` (was `FrontFaceUndefined`). Enum values renumbered: CCW=0, CW=1. Removed `FrontFaceUndefined`.
- **CullMode**: zero value is now `CullModeNone` (was `CullModeUndefined`). Enum values renumbered: None=0, Front=1, Back=2. Removed `CullModeUndefined`.
- **`DefaultPrimitiveState()`** now returns `PrimitiveState{}` — the zero value IS the WebGPU spec default. Function kept for explicit documentation and parity with other Default*State helpers.

**Why:** Go zero value of `PrimitiveState{}` is now a fully valid WebGPU-spec-default configuration. No normalization pass needed. Eliminates the class of bugs where `Undefined` sentinel values leak into HAL backends.

**Migration:** downstream repos (wgpu, gogpu, gg) must update HAL enum→API mapping tables. `*Undefined` constants no longer exist — remove any references or `switch` cases.

## [v0.4.0] - 2026-04-03

### Added

- **BlendComponent.UsesConstant()** — returns true if the blend component uses
  `BlendFactorConstant` or `BlendFactorOneMinusConstant` in either `SrcFactor` or
  `DstFactor`. Used by wgpu for draw-time validation that `SetBlendConstant()` has
  been called when the pipeline requires it. Matches Rust wgpu-types
  `BlendComponent::uses_constant()`.

## [v0.3.0] - 2026-03-10

### Added

- **TextureUsage.ContainsUnknownBits()** — returns true if the usage contains
  any unknown flags. Follows the same pattern as `BufferUsage.ContainsUnknownBits()`.
  Used by wgpu core validation layer for texture descriptor validation.

## [v0.2.0] - 2026-01-29

### Changed

- **webgpu.h compliance**: All enum values now use explicit hex constants matching the official WebGPU C header specification
  - TextureFormat: 97 formats with spec-compliant values (0x00000000 - 0x00000060)
  - BufferUsage: Explicit bit flags matching WebGPU spec
  - TextureUsage: Explicit bit flags matching WebGPU spec
  - LoadOp, StoreOp: Spec-compliant values
  - BlendFactor, BlendOperation: Spec-compliant values
  - AddressMode, FilterMode: Spec-compliant values
  - VertexFormat: 31 formats with spec-compliant values
  - PresentMode, CompositeAlphaMode: Spec-compliant values

### Migration

Values changed from iota-based to explicit webgpu.h values. This ensures:
- Binary compatibility with wgpu-native and other WebGPU implementations
- Correct serialization/deserialization of GPU descriptors
- Interoperability with C/Rust WebGPU code

If you were relying on specific numeric values, they may have changed. Use the named constants.

## [v0.1.0] - 2026-01-29

### Added

- Initial release with WebGPU type definitions
- **Texture types**: TextureFormat (97 formats including BC, ETC2, ASTC), TextureUsage, TextureDimension, TextureViewDimension, TextureAspect, TextureSampleType
- **Buffer types**: BufferUsage, BufferBindingType, BufferDescriptor, IndexFormat
- **Sampler types**: AddressMode, FilterMode, MipmapFilterMode, CompareFunction, SamplerDescriptor
- **Render types**: LoadOp, StoreOp, BlendState, BlendFactor, BlendOperation, ColorWriteMask, RenderPassDescriptor
- **Shader types**: ShaderStage, ShaderSource (WGSL, SPIR-V, GLSL)
- **Vertex types**: VertexFormat (31 formats), VertexStepMode, VertexBufferLayout
- **Binding types**: BindGroupLayoutEntry, BindGroupEntry, BufferBindingLayout, TextureBindingLayout
- **Adapter types**: DeviceType, Backend, PowerPreference, AdapterInfo
- **Surface types**: PresentMode, CompositeAlphaMode, SurfaceConfiguration
- **Limits & Features**: Full WebGPU limits struct, feature flags
- **Geometry types**: Extent3D, Origin3D, Color
