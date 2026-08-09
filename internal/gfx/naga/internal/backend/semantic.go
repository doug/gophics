// Package backend holds cross-backend constants and helpers shared between
// naga's text backends (HLSL) and binary backends (DXIL) that both target
// the D3D ecosystem. Centralizing these values here prevents the HLSL
// writer and the DXIL emitter from drifting — a drift that directly
// causes D3D12 pipeline-state creation failures at the input-layout
// boundary (see BUG-DXIL-028).
//
// Anything placed here must be:
//   - a spec-level output convention, not IR semantics
//   - referenced by at least two sibling backends that otherwise would
//     duplicate a literal
//   - stable enough that external tooling (wgpu/hal, gg, ui) can rely on it
package backend

// LocationSemantic is the semantic-name prefix for user-defined
// @location(N) vertex-shader inputs and inter-stage varyings in both
// HLSL source and DXIL input/output signatures.
//
// It is a naga ecosystem convention. DXIL does not prescribe a name for
// arbitrary (non-SV_*) semantics; the backends choose one and every
// consumer must agree.
//
// Historical consumers were the HLSL/DXIL backends and wgpu/hal/dx12
// (removed 2026-08 with the DirectX chain — Windows renders via Vulkan);
// the constant stays as the naga ecosystem convention for any future
// signature-emitting backend.
//
// A mismatch between the DXIL input signature and the D3D12 input layout
// causes CreateGraphicsPipelineState to return E_INVALIDARG. IDxcValidator
// cannot detect this because the DXIL container is internally consistent;
// the break only surfaces at the container-to-pipeline boundary.
//
// See BUG-DXIL-028 for the discovery trail.
const LocationSemantic = "LOC"
