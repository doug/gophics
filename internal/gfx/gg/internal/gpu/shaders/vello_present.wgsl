// vello_present.wgsl — copies the compute pipeline's packed output into a
// storage texture, so the result can be composited on the GPU.
//
// The fine stage writes premultiplied RGBA packed one pixel per u32
// (R | G<<8 | B<<16 | A<<24). Presenting that meant reading the whole buffer
// back to host memory and compositing in a CPU loop, which is both the
// dominant cost of a compute frame and impossible for a target that has no CPU
// buffer at all.
//
// This exists rather than a CopyBufferToTexture because WebGPU requires the
// bytes-per-row of a buffer-to-texture copy to be a multiple of 256, and the
// output is tightly packed at width*4 — 400 bytes for a 100px-wide target.
// Padding every row to satisfy that would mean changing how the fine stage
// indexes its output, which is the one part of this pipeline that is now
// verified stage by stage against the CPU port.

struct Config {
    width: u32,
    height: u32,
}

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> packed: array<u32>;
@group(0) @binding(2) var dst: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(8, 8, 1)
fn cs_present(@builtin(global_invocation_id) gid: vec3<u32>) {
    if gid.x >= config.width || gid.y >= config.height {
        return;
    }
    let v = packed[gid.y * config.width + gid.x];
    let rgba = vec4<f32>(
        f32(v & 0xffu),
        f32((v >> 8u) & 0xffu),
        f32((v >> 16u) & 0xffu),
        f32((v >> 24u) & 0xffu),
    ) / 255.0;
    textureStore(dst, vec2<i32>(i32(gid.x), i32(gid.y)), rgba);
}
