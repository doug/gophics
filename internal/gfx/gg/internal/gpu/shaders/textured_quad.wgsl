// textured_quad.wgsl - Textured Quad Rendering Shader (Tier 3)
//
// Renders image patterns as textured quads with full affine transform support.
// The vertex shader applies an ortho projection (pixel-to-NDC), and the
// fragment shader samples the image texture with bilinear filtering.
//
// Premultiplied alpha throughout: the source image is expected in premultiplied
// RGBA format, and the fragment shader applies opacity as a uniform multiplier.
//
// NOTE: All math avoids naga-problematic builtins (smoothstep, clamp, abs,
// min, max, select). Only sqrt() is used where needed.
//
// References:
// - Skia GrFillRectOp + GrTextureProxy (textured quad is fundamental GPU op)
// - Qt Quick QSGSimpleTextureNode (basic compositing primitive)
// - Vello DrawImage (scene command → atlas → textured quad)

struct ImageUniforms {
    transform: mat4x4<f32>,  // ortho projection matrix
    opacity: f32,            // opacity multiplier (0.0 to 1.0)
    // Three scalar pads, NOT vec3<f32>: a vec3 aligns to 16 bytes, so it would be
    // placed at offset 80 (after opacity at 64) and round the struct up to 96
    // bytes. The Go side (imageUniformSize = 80, makeImageUniform) writes opacity
    // at offset 64 and zero-pads bytes 68..79 — an 80-byte layout. The vec3 made
    // the shader expect 96, so Metal's validation aborted every image/sprite draw:
    // "argument uniforms[0] ... has space for 80 bytes, but argument has a
    // length(96)". Three f32s keep the struct at exactly 80 bytes.
    // FROST-BLUR (VULKAN-VERIFY): repurposed from _pad0.._pad2 — the 80-byte
    // layout is unchanged (three f32s, same as the pads), so the Metal validation
    // note above still holds. All three are 0 for normal image/sprite draws, so
    // fs_main takes the plain single-sample, no-op-saturation path for those.
    saturation: f32,   // >0 pushes color away from its luma (glass vibrancy); 0 = unchanged
    blur_step_x: f32,  // per-tap UV offset of a separable Gaussian; 0/0 = single sample
    blur_step_y: f32,
}

struct VertexInput {
    @location(0) position: vec2<f32>,   // quad corner in pixel coords
    @location(1) tex_coord: vec2<f32>,  // UV coordinates (0..1 range)
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) tex_coord: vec2<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: ImageUniforms;
@group(0) @binding(1) var image_texture: texture_2d<f32>;
@group(0) @binding(2) var image_sampler: sampler;

// --- RRect clip uniform (shared across all pipelines) ---
struct ClipParams {
    clip_rect: vec4<f32>,   // (left, top, right, bottom) device pixels
    clip_radius: f32,
    clip_enabled: f32,      // 0.0 = no clip, 1.0 = active
    _pad: vec2<f32>,
}
@group(1) @binding(0) var<uniform> clip: ClipParams;

fn rrect_clip_coverage(frag_pos: vec2<f32>) -> f32 {
    // Image shaders: no per-pixel SDF clip. Returns 1.0.
    //
    // Same reasoning as glyph_mask.wgsl: textureSample combined with
    // complex SDF math causes Intel Vulkan register pressure issues.
    // Image clipping is handled by hardware scissor (GPU-CLIP-001).
    // RRect clip for images will use stencil buffer (GPU-CLIP-003).
    return 1.0;
}

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    out.position = uniforms.transform * vec4<f32>(in.position, 0.0, 1.0);
    out.tex_coord = in.tex_coord;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    let uv = in.tex_coord;
    let step = vec2<f32>(uniforms.blur_step_x, uniforms.blur_step_y);

    // FROST-BLUR (VULKAN-VERIFY): one leg of a separable Gaussian (13 taps,
    // sigma ~2 in tap space, weights normalized to sum 1). appendBackdropBlur
    // runs this twice — a horizontal pass then a vertical pass — over the
    // downsampled backdrop; the Go side sizes `step` so ±6 taps span the blur
    // radius. This is a real blur KERNEL (matches the CPU box-blur look), not a
    // mip-pyramid upscale. It runs ONLY for the frost composite: `step` is a
    // uniform, so the branch never diverges within a draw, and normal draws
    // (step == 0) skip straight to the single sample below — zero behavior change
    // for them. The cost is that these taps compile into the shared image shader;
    // if a constrained backend (Intel Vulkan register pressure — see the header)
    // regresses, hoist this into a dedicated blur pipeline. The Go side keeps the
    // passes isolated (their own render targets) so that move is mechanical.
    var texel: vec4<f32>;
    if (step.x != 0.0 || step.y != 0.0) {
        var acc = textureSample(image_texture, image_sampler, uv) * 0.199676;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step)
                   + textureSample(image_texture, image_sampler, uv - step)) * 0.176216;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step * 2.0)
                   + textureSample(image_texture, image_sampler, uv - step * 2.0)) * 0.121110;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step * 3.0)
                   + textureSample(image_texture, image_sampler, uv - step * 3.0)) * 0.064824;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step * 4.0)
                   + textureSample(image_texture, image_sampler, uv - step * 4.0)) * 0.027024;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step * 5.0)
                   + textureSample(image_texture, image_sampler, uv - step * 5.0)) * 0.008773;
        acc = acc + (textureSample(image_texture, image_sampler, uv + step * 6.0)
                   + textureSample(image_texture, image_sampler, uv - step * 6.0)) * 0.002218;
        texel = acc;
    } else {
        texel = textureSample(image_texture, image_sampler, uv);
    }

    // FROST-BLUR (VULKAN-VERIFY): saturation boost for the glass "vibrancy" — the
    // blurred backdrop otherwise reads grey/muted. Work in straight-alpha space
    // (un-premultiply, adjust, re-premultiply) so alpha is untouched; sat == 0
    // (all normal draws) skips it entirely. Over-range results are clamped by the
    // render target on write, so no clamp() (a naga-problematic builtin) is used.
    let sat = uniforms.saturation;
    if (sat > 0.0) {
        var a = texel.a;
        var rgb = texel.rgb;
        if (a > 0.0) { rgb = rgb / a; }
        let luma = dot(rgb, vec3<f32>(0.2126, 0.7152, 0.0722));
        rgb = vec3<f32>(luma) + (rgb - vec3<f32>(luma)) * sat;
        texel = vec4<f32>(rgb * a, a);
    }

    let clip_cov = rrect_clip_coverage(in.position.xy);
    // Apply opacity to premultiplied texel (scale all channels uniformly).
    let opacity = uniforms.opacity * clip_cov;
    return texel * opacity;
}
