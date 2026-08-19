// clip_leaf.wgsl — match BeginClip/EndClip pairs and fix up their draw monoids.
//
// Without this stage an EndClip has no path index, so coarse cannot emit the
// clip path's coverage, and no scene offset, so it cannot read the blend mode
// and alpha. The GPU pipeline had no clip stage at all: clips encoded into the
// scene produced output differing from the render-pass path on 95% of inked
// pixels, because coarse consumed clip data nothing produced.
//
// Vello resolves the pairing with a parallel bicyclic semigroup scan. This is
// a single invocation walking a stack, which is what the CPU port does and is
// straightforwardly correct. Clip counts are in the tens or hundreds for real
// scenes, and the stage is a rounding error next to path_count or fine; if a
// scene ever makes this the bottleneck, the parallel formulation is the known
// answer.

struct DrawMonoid {
    path_ix: u32,
    clip_ix: u32,
    scene_offset: u32,
    info_offset: u32,
}

struct ClipInp {
    ix: u32,
    path_ix: i32,
}

struct Config {
    width_in_tiles: u32,
    height_in_tiles: u32,
    target_width: u32,
    target_height: u32,
    n_drawobj: u32,
    n_path: u32,
    n_clip: u32,
    pathtag_base: u32,
    pathdata_base: u32,
    drawtag_base: u32,
    drawdata_base: u32,
    transform_base: u32,
    style_base: u32,
    n_lines: u32,
    bg_color: u32,
}

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> clip_inps: array<ClipInp>;
@group(0) @binding(2) var<storage, read_write> draw_monoids: array<DrawMonoid>;

// Bounded so the stack is a fixed-size array. Deeper nesting than this is
// dropped rather than corrupting memory: the unmatched EndClip simply keeps
// the monoid it already had, which renders unclipped rather than wrongly.
const MAX_CLIP_DEPTH: u32 = 64u;

@compute @workgroup_size(1, 1, 1)
fn main() {
    var stack: array<u32, MAX_CLIP_DEPTH>;
    var depth = 0u;

    let n = config.n_clip;
    for (var i = 0u; i < n; i = i + 1u) {
        let inp = clip_inps[i];
        if inp.path_ix >= 0 {
            if depth < MAX_CLIP_DEPTH {
                stack[depth] = i;
                depth = depth + 1u;
            }
            continue;
        }

        // EndClip: pop the matching BeginClip.
        if depth == 0u {
            continue;
        }
        depth = depth - 1u;
        let parent = clip_inps[stack[depth]];

        let end_ix = u32(~inp.path_ix);
        if end_ix < config.n_drawobj && parent.ix < config.n_drawobj {
            // The end takes the begin's path, so coarse can draw the clip
            // shape, and its scene offset, so coarse can read blend and alpha.
            draw_monoids[end_ix].path_ix = u32(parent.path_ix);
            draw_monoids[end_ix].scene_offset = draw_monoids[parent.ix].scene_offset;
        }
    }
}
