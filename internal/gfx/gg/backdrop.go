package gg

import "math"

// BackdropBlur blurs the already-rendered content within the logical rect
// (x,y,w,h), by the given radius, in place — the core of a frosted-glass
// material. It reads and writes the CPU pixmap, so it takes effect on the CPU
// rasterizer: the reference renderer, all headless/offscreen renders, and the
// mobile CPU-fallback present path. On a GPU-accelerated context the pixmap is
// not the live backdrop, so this is a no-op (a caller's translucent tint stands
// in for the glass) until GPU backdrop-texture sampling lands.
//
// The rect is in the current user space; the active transform (e.g. a Canvas
// box's translate) maps it to device pixels, matching every other draw call.
func (c *Context) BackdropBlur(x, y, w, h, radius float64) {
	if radius <= 0 || w <= 0 || h <= 0 {
		return
	}
	if c.gpuCtxOps() != nil {
		return // GPU direct path: c.pixmap isn't the composited backdrop
	}
	pm := c.pixmap
	if pm == nil {
		return
	}
	// Map the user-space rect to device pixels via the current transform.
	m := c.totalMatrix()
	tl := m.TransformPoint(Pt(x, y))
	br := m.TransformPoint(Pt(x+w, y+h))
	x0 := int(math.Round(math.Min(tl.X, br.X)))
	y0 := int(math.Round(math.Min(tl.Y, br.Y)))
	x1 := int(math.Round(math.Max(tl.X, br.X)))
	y1 := int(math.Round(math.Max(tl.Y, br.Y)))
	r := int(math.Round(radius * c.deviceScale))
	blurPixmapRegion(pm.data, pm.width, pm.height, x0, y0, x1-x0, y1-y0, r)
}

// blurPixmapRegion applies a 3-pass separable box blur (a close, fast
// approximation of a Gaussian) to the [x,y,w,h] region of a premultiplied-RGBA
// pixmap. It reads neighbouring pixels up to the kernel radius — clamped to the
// pixmap, and padded beyond the region — so the region's edges blend with the
// surrounding backdrop instead of darkening.
func blurPixmapRegion(data []uint8, W, H, x, y, w, h, radius int) {
	if radius < 1 {
		return
	}
	x0, y0 := blurClamp(x, 0, W), blurClamp(y, 0, H)
	x1, y1 := blurClamp(x+w, 0, W), blurClamp(y+h, 0, H)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	// Working buffer: the region padded by 3*radius (enough for three box
	// passes), clamped to the pixmap, so edge pixels average real backdrop.
	pad := radius * 3
	bx0, by0 := blurClamp(x0-pad, 0, W), blurClamp(y0-pad, 0, H)
	bx1, by1 := blurClamp(x1+pad, 0, W), blurClamp(y1+pad, 0, H)
	bw, bh := bx1-bx0, by1-by0
	buf := make([]uint8, bw*bh*4)
	for row := 0; row < bh; row++ {
		src := ((by0+row)*W + bx0) * 4
		copy(buf[row*bw*4:(row+1)*bw*4], data[src:src+bw*4])
	}
	tmp := make([]uint8, len(buf))
	for i := 0; i < 3; i++ {
		boxBlurH(buf, tmp, bw, bh, radius)
		buf, tmp = tmp, buf
		boxBlurV(buf, tmp, bw, bh, radius)
		buf, tmp = tmp, buf
	}
	// Write back only the original region.
	for py := y0; py < y1; py++ {
		srcRow := (py - by0) * bw * 4
		dstRow := py * W * 4
		copy(data[dstRow+x0*4:dstRow+x1*4], buf[srcRow+(x0-bx0)*4:srcRow+(x1-bx0)*4])
	}
}

// boxBlurH averages each pixel over a horizontal window of 2r+1, per channel,
// with a sliding sum; edges clamp. src and dst are bw*bh premultiplied RGBA.
func boxBlurH(src, dst []uint8, w, h, r int) {
	win := 2*r + 1
	for y := 0; y < h; y++ {
		base := y * w * 4
		for ch := 0; ch < 4; ch++ {
			sum := 0
			for k := -r; k <= r; k++ {
				sum += int(src[base+blurClamp(k, 0, w-1)*4+ch])
			}
			for x := 0; x < w; x++ {
				dst[base+x*4+ch] = uint8(sum / win)
				sum += int(src[base+blurClamp(x+r+1, 0, w-1)*4+ch]) -
					int(src[base+blurClamp(x-r, 0, w-1)*4+ch])
			}
		}
	}
}

// boxBlurV is boxBlurH along the vertical axis.
func boxBlurV(src, dst []uint8, w, h, r int) {
	win := 2*r + 1
	stride := w * 4
	for x := 0; x < w; x++ {
		base := x * 4
		for ch := 0; ch < 4; ch++ {
			sum := 0
			for k := -r; k <= r; k++ {
				sum += int(src[base+blurClamp(k, 0, h-1)*stride+ch])
			}
			for y := 0; y < h; y++ {
				dst[base+y*stride+ch] = uint8(sum / win)
				sum += int(src[base+blurClamp(y+r+1, 0, h-1)*stride+ch]) -
					int(src[base+blurClamp(y-r, 0, h-1)*stride+ch])
			}
		}
	}
}

func blurClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
