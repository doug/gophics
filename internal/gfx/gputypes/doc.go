// Package gputypes provides WebGPU type definitions for the gogpu ecosystem.
//
// gputypes is the single source of truth for WebGPU types, providing all enums,
// structs, and constants used across the ecosystem. It has ZERO external dependencies,
// making it the foundation package that all other packages can safely import.
//
// # Architecture
//
// gputypes sits at the bottom of the dependency graph:
//
//	              gputypes (zero deps)
//	                     │
//	     ┌───────────────┼───────────────┐
//	     ▼               ▼               ▼
//	gpucontext         wgpu       go-webgpu/webgpu
//	     │               │               │
//	     └───────┬───────┴───────┬───────┘
//	             ▼               ▼
//	          gogpu           born-ml
//
// # Type Categories
//
// The package provides types in several categories:
//
// Texture types: TextureFormat, TextureUsage, TextureDimension, TextureDescriptor, etc.
//
// Buffer types: BufferUsage, BufferDescriptor, IndexFormat, etc.
//
// Sampler types: AddressMode, FilterMode, CompareFunction, SamplerDescriptor, etc.
//
// Render types: BlendState, BlendFactor, BlendOperation, PrimitiveTopology, etc.
//
// Shader types: ShaderStage, ShaderModuleDescriptor, etc.
//
// Vertex types: VertexFormat, VertexStepMode, VertexAttribute, etc.
//
// Binding types: BindGroupLayoutEntry, BufferBindingLayout, TextureBindingLayout, etc.
//
// Adapter types: DeviceType, AdapterInfo, PowerPreference, Backend, etc.
//
// Limits and Features: Limits struct, Features flags, etc.
//
// Geometry types: Extent3D, Origin3D, Color, etc.
//
// # Usage
//
// Import the package and use types directly:
//
//	import "github.com/doug/gophics/internal/gfx/gputypes"
//
//	format := gputypes.TextureFormatRGBA8Unorm
//	usage := gputypes.TextureUsageCopySrc | gputypes.TextureUsageRenderAttachment
//
//	desc := gputypes.TextureDescriptor{
//	    Size:   gputypes.Extent3D{Width: 800, Height: 600, DepthOrArrayLayers: 1},
//	    Format: format,
//	    Usage:  usage,
//	}
//
// # WebGPU Specification
//
// Types follow the W3C WebGPU specification:
// https://www.w3.org/TR/webgpu/
//
// Naming conventions follow wgpu-types (Rust) where applicable:
// https://docs.rs/wgpu-types
package gputypes
