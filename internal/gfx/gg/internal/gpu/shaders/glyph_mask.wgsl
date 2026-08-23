// glyph_mask.wgsl - Alpha Mask Text Rendering Shader (Tier 6)
//
// Renders CPU-rasterized glyph alpha masks as textured quads. The atlas
// stores R8 (single-channel) coverage data produced by AnalyticFiller.
//
// The fragment shader outputs premultiplied alpha.
// Color is passed via uniform buffer (per-batch).
//
// References:
// - Skia GrAtlasTextOp (R8 atlas compositing)
// - Chrome cc::GlyphAtlas (alpha mask cache + GPU upload)

struct GlyphMaskUniforms {
    transform: mat4x4<f32>,
    color: vec4<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,
    @location(1) tex_coord: vec2<f32>,
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) tex_coord: vec2<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: GlyphMaskUniforms;
@group(0) @binding(1) var atlas_texture: texture_2d<f32>;
@group(0) @binding(2) var atlas_sampler: sampler;

// --- RRect clip uniform (shared across all pipelines) ---
struct ClipParams {
    clip_rect: vec4<f32>,
    clip_radius: f32,
    clip_enabled: f32,
    _pad: vec2<f32>,
}
@group(1) @binding(0) var<uniform> clip: ClipParams;

fn rrect_clip_coverage(frag_pos: vec2<f32>) -> f32 {
    // Text shaders: no per-pixel SDF clip. Returns 1.0 (no clipping).
    //
    // Enterprise research (GPU-CLIP-002) found that NO production 2D engine
    // (Vello, Skia Graphite/Ganesh, Pathfinder, Qt RHI) computes per-pixel
    // SDF clip inside text fragment shaders. The industry-standard approach
    // is stencil-buffer clip (Skia Ganesh) or depth-buffer clip (Graphite).
    //
    // Per-pixel SDF clip (11 sqrt calls) combined with textureSample causes
    // Intel Vulkan shader compiler to generate corrupt code — text becomes
    // invisible. This is a known Intel driver limitation with register
    // pressure from complex ALU + texture sampling in the same shader.
    //
    // Text clipping is handled by:
    //   1. Hardware scissor rect (axis-aligned, free) — GPU-CLIP-001
    //   2. Stencil-buffer RRect clip (planned) — GPU-CLIP-003
    //
    // The @group(1) binding is kept for uniform pipeline layout across all
    // tiers, avoiding per-tier bind group logic in GPURenderSession.
    return 1.0;
}

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    // Plain matrix-vector multiply.
    //
    // This used to expand the columns by hand — transform[0]..transform[3],
    // each scaled and summed — to work around naga's SPIR-V backend not
    // supporting mat4x4 * vec4 as one operator. That support exists now
    // (OpMatrixTimesVector, spirv/internal/codegen/backend.go), so the
    // workaround outlived its reason, and on Adreno 619 / Android 16 it
    // miscompiled: the vertex stage produced a degenerate position, every
    // glyph quad collapsed, and text vanished while shapes drew correctly.
    // Metal was unaffected, which is why it only ever showed on device.
    //
    // The same expansion was in glyph_mask_lcd, msdf_text and textured_quad —
    // exactly the tiers that were reported blank on that device. Do not
    // reintroduce it without re-checking the backend first.
    out.position = uniforms.transform * vec4<f32>(in.position, 0.0, 1.0);
    out.tex_coord = in.tex_coord;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    // Glyph masks are pre-rasterized at device resolution into a SINGLE-MIP R8
    // atlas, so they must be sampled at mip 0. Use textureSampleLevel(..., 0.0),
    // NOT textureSample(): textureSample derives the LOD from screen-space
    // derivatives, and on tile-based mobile GPUs (verified on Imagination
    // PowerVR / Pixel, Vulkan) minified fragments (LOD>0) sampling a single-mip
    // texture come back as ~1.0 — every glyph renders as a solid block. Forcing
    // LOD 0 is both correct (no mip chain exists) and fixes the mobile blocker.
    // Desktop Metal was unaffected, which is why this only showed on device.
    let alpha = textureSampleLevel(atlas_texture, atlas_sampler, in.tex_coord, 0.0).r;
    let clip_cov = rrect_clip_coverage(in.position.xy);
    let color = uniforms.color;
    let a = alpha * color.a * clip_cov;
    return vec4<f32>(color.rgb * a, a);
}
