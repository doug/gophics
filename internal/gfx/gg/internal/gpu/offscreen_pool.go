package gpu

import (
	"sync"

	"github.com/doug/gophics/internal/gfx/wgpu"
)

// offscreenPool reuses the offscreen textures the layer and backdrop-blur
// passes render into.
//
// Those passes are not shy about allocating. One backdrop blur takes a
// full-surface capture of everything drawn behind it, downsamples it in
// halving steps, runs two Gaussian passes and composites — six textures,
// created and destroyed every frame it is on screen, the largest of them the
// whole surface. On a desktop GPU that is affordable and on a phone it is not:
// a single blur measured p95 53ms against 7.5ms for a frame without one, on an
// A15 whose memory bandwidth is a fraction of a desktop part's.
//
// The sizes repeat exactly frame to frame, because they are derived from the
// surface and the blur radius rather than from anything that varies. So the
// textures are handed back instead of destroyed, and the next frame takes the
// same ones. Nothing about what is rendered changes.
//
// Reuse is safe at exactly the point the old code destroyed: resolveLayers
// frees the previous frame's textures at the top of the next one, by which
// time the submit that sampled them has completed.
type offscreenPool struct {
	mu    sync.Mutex
	free  map[offscreenKey][]offscreenTex
	bytes int64 // total held, against poolByteBudget
}

type offscreenKey struct{ w, h int }

type offscreenTex struct {
	tex  *wgpu.Texture
	view *wgpu.TextureView
}

// The pool is bounded by bytes, not by count.
//
// A count is the wrong shape here because the sizes differ by orders of
// magnitude: the full-surface capture is about 11MB at 1120x2432 while the
// third downsample is under 200KB. Four of each sounds modest and is 44MB of
// the large ones — retained, on a phone, where it was previously handed back
// every frame. Trading a frame's allocation for a permanent 44MB reservation
// is not obviously a good deal, and on a device under memory pressure it is
// clearly a bad one.
//
// So: a total budget, and the largest textures simply do not fit many copies
// of themselves into it. 24MB holds several of everything a normal frame asks
// for, and about two full-surface captures.
const (
	poolByteBudget = 24 << 20
	perSizeCap     = 4 // still bounded per size, so one size cannot own the budget
)

// bytesFor is the memory a w×h BGRA8 texture occupies.
func bytesFor(w, h int) int64 { return int64(w) * int64(h) * 4 }

// take returns a pooled texture for w×h, or false when none is available.
func (p *offscreenPool) take(w, h int) (offscreenTex, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := offscreenKey{w, h}
	n := len(p.free[k])
	if n == 0 {
		return offscreenTex{}, false
	}
	t := p.free[k][n-1]
	p.free[k] = p.free[k][:n-1]
	p.bytes -= bytesFor(w, h)
	return t, true
}

// put hands a texture back, or destroys it when the size is already well
// stocked. Reports whether it was kept.
func (p *offscreenPool) put(w, h int, t offscreenTex) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.free == nil {
		p.free = map[offscreenKey][]offscreenTex{}
	}
	k := offscreenKey{w, h}
	sz := bytesFor(w, h)
	if len(p.free[k]) >= perSizeCap || p.bytes+sz > poolByteBudget {
		return false
	}
	p.free[k] = append(p.free[k], t)
	p.bytes += sz
	return true
}

// destroyAll empties the pool. For teardown, and for a surface resize, after
// which the old sizes will never be asked for again.
func (p *offscreenPool) destroyAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, ts := range p.free {
		for _, t := range ts {
			t.view.Release()
			t.tex.Release()
		}
		delete(p.free, k)
	}
	p.bytes = 0
}
