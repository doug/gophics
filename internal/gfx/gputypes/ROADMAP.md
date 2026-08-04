# Roadmap

## Current: v0.5.0

Go zero values for PrimitiveState enums = WebGPU spec defaults. `PrimitiveState{}` is a valid configuration.

## Released

### v0.5.0 (2026-04-21)
- **BREAKING:** PrimitiveTopology, FrontFace, CullMode renumbered — zero value = WebGPU spec default
- Removed `*Undefined` sentinel constants
- `DefaultPrimitiveState()` now returns `PrimitiveState{}` (zero value)

### v0.4.0 (2026-04-03)
- `BlendComponent.UsesConstant()` for blend constant validation

### v0.3.0 (2026-03-10)
- `TextureUsage.ContainsUnknownBits()` for validation

### v0.2.0 (2026-01-29)
- webgpu.h spec-compliant enum values
- Binary compatibility with wgpu-native

### v0.1.0 (2026-01-29)
- Initial release with core WebGPU types

## Planned

### v1.0.0
- Stable API matching WebGPU spec
- Full coverage of all WebGPU types
- Comprehensive documentation

## Design Principles

1. **Zero dependencies** — Always
2. **WebGPU spec compliance** — Follow W3C WebGPU exactly
3. **Go-idiomatic zero values** — Zero value of any type should be the spec default
4. **Ecosystem compatibility** — Works with all gogpu projects
