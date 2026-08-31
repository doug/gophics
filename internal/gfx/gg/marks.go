package gg

import (
	"math"
	"sync"

	"github.com/doug/gophics/internal/gfx/gg/internal/clip"
)

// MarkShape is the repeated shape a BlitMarks batch draws.
type MarkShape uint8

const (
	// MarkCircle is a filled circle of the given diameter.
	MarkCircle MarkShape = iota
	// MarkSquare is an axis-aligned filled square of the given side.
	MarkSquare
)

// markStamp is a coverage mask for one shape at one device size. Cov is
// row-major W*H bytes of 0..255 coverage; off locates the shape's centre, so a
// mark centred at device (cx, cy) writes its top-left at (cx-off, cy-off).
type markStamp struct {
	cov  []uint8
	w, h int
	off  int
}

// markStampKey caches by shape, device diameter quantized to eighths of a
// pixel, and the mark's sub-pixel offset quantized to quarters.
//
// The sub-pixel part matters as much as the size. A stamp built for one phase
// and blitted at another lands up to half a pixel off, which on a 3px dot is a
// sixth of its width — and against the analytic rasterizer it shows as a
// wholly different edge. Quantizing to quarters costs 16 stamps per size and
// is the same trade the glyph atlas makes for exactly the same reason.
type markStampKey struct {
	shape    MarkShape
	sizeQ    int
	fxQ, fyQ int
}

// markStampFor builds or returns the cached coverage mask for one device size.
//
// Built once and reused for every mark at that size, which is the whole point:
// the expensive part of a mark is deciding its coverage, and that answer is
// identical for marks differing only in position and colour.
func markStampFor(shape MarkShape, deviceD, fx, fy float64) *markStamp {
	q := int(math.Round(deviceD * 8))
	if q < 1 {
		return nil
	}
	fxQ, fyQ := int(fx*4), int(fy*4)
	fxQ, fyQ = min(max(fxQ, 0), 3), min(max(fyQ, 0), 3)
	key := markStampKey{shape: shape, sizeQ: q, fxQ: fxQ, fyQ: fyQ}
	markStampMu.Lock()
	defer markStampMu.Unlock()
	if s, ok := markStamps[key]; ok {
		return s
	}
	s := buildMarkStamp(shape, float64(q)/8, float64(fxQ)/4, float64(fyQ)/4)
	markStamps[key] = s
	return s
}

var (
	markStampMu sync.Mutex
	markStamps  = map[markStampKey]*markStamp{}
)

// buildMarkStamp rasterizes one mark by supersampling. Sampling rather than an
// analytic fill because it happens once per size and is then reused thousands
// of times; a 4x4 grid holds the edge within a sixteenth of full coverage,
// which at mark sizes is below a visible step.
func buildMarkStamp(shape MarkShape, d, fx, fy float64) *markStamp {
	r := d / 2
	off := int(math.Ceil(r)) + 1
	w := 2*off + 2
	cov := make([]uint8, w*w)
	const sub, subN = 4, 16
	// Stamp pixel px covers [px, px+1); the mark's centre sits at off+fx, so
	// placing pixel 0 at floor(cx)-off puts the centre back at exactly cx.
	for py := range w {
		for px := range w {
			hits := 0
			for sy := range sub {
				for sx := range sub {
					x := float64(px) + (float64(sx)+0.5)/sub - (float64(off) + fx)
					y := float64(py) + (float64(sy)+0.5)/sub - (float64(off) + fy)
					in := x*x+y*y <= r*r
					if shape == MarkSquare {
						in = math.Abs(x) <= r && math.Abs(y) <= r
					}
					if in {
						hits++
					}
				}
			}
			cov[py*w+px] = uint8(hits * 255 / subN)
		}
	}
	return &markStamp{cov: cov, w: w, h: w, off: off}
}

// BlitMarks composites stamp at each of the given user-space centres, in cols,
// straight into the pixmap. It reports whether it did; false means the caller
// must fall back to filling each mark as a path.
//
// This exists because drawing many small identical shapes through the path
// rasterizer spends nearly all its time deciding coverage — building edges,
// flattening curves, sweeping scanlines — and that answer is the same for every
// mark. Profiling a 10,000-point scatter put ~75% of the frame there, and
// routing the same marks through DrawImage did not help, because image drawing
// is itself a rectangle fill with a pattern paint and lands back in the same
// rasterizer. Only writing spans directly skips it.
//
// It declines rather than approximates whenever the general path would differ:
//
//   - A rotated or skewed transform. The stamp is axis-aligned; under shear a
//     circle is an ellipse and the mask is simply the wrong shape.
//   - A non-uniform or non-positive scale, for the same reason.
//   - An active GPU accelerator. The GPU has its own batching and the pixmap
//     here is not what reaches the screen.
//   - Anything but SourceOver, or a global alpha, or a soft (non-rectangular)
//     clip. Each is a compositing rule this loop does not implement, and
//     silently ignoring one would be worse than being slow.
func (c *Context) BlitMarks(shape MarkShape, diameter float64, xs, ys []float32, cols []RGBA) bool {
	if diameter <= 0 {
		return false
	}
	n := len(xs)
	if n == 0 || len(ys) != n || len(cols) != n {
		return false
	}
	if c.pixmap == nil {
		return false
	}
	// The GPU keeps its own target; this pixmap is not what reaches the screen.
	if Accelerator() != nil && !c.GPUDisabled() {
		return false
	}
	// An alpha mask is per-pixel coverage this loop has nowhere to read.
	if c.mask != nil {
		return false
	}

	m := c.totalMatrix()
	// Axis-aligned, uniform, positive scale only. B and D are the shear terms.
	if m.B != 0 || m.D != 0 || m.A <= 0 || m.E <= 0 || math.Abs(m.A-m.E) > 1e-6 {
		return false
	}

	// A rectangular clip is honoured by intersecting bounds; anything softer
	// would need per-pixel coverage this loop does not read.
	clipLo, clipTo, clipHi, clipBo, ok := c.rectClipBounds()
	if !ok {
		return false
	}

	pw, ph := c.pixmap.Width(), c.pixmap.Height()
	if clipLo < 0 {
		clipLo = 0
	}
	if clipTo < 0 {
		clipTo = 0
	}
	if clipHi > pw {
		clipHi = pw
	}
	if clipBo > ph {
		clipBo = ph
	}

	deviceD := diameter * m.A
	for i := range n {
		// User space to device pixels, then to the mask's top-left. The
		// fractional part selects the stamp phase rather than being rounded
		// away, so a mark lands where it was asked to.
		cx := float64(xs[i])*m.A + m.C
		cy := float64(ys[i])*m.E + m.F
		fx, fy := cx-math.Floor(cx), cy-math.Floor(cy)
		stamp := markStampFor(shape, deviceD, fx, fy)
		if stamp == nil {
			return false
		}
		x0 := int(math.Floor(cx)) - stamp.off
		y0 := int(math.Floor(cy)) - stamp.off

		// Clip the stamp rectangle to the writable region, so the inner loops
		// carry no bounds tests.
		sx0, sy0 := 0, 0
		sx1, sy1 := stamp.w, stamp.h
		if d := clipLo - x0; d > 0 {
			sx0 = d
		}
		if d := clipTo - y0; d > 0 {
			sy0 = d
		}
		if d := (x0 + stamp.w) - clipHi; d > 0 {
			sx1 -= d
		}
		if d := (y0 + stamp.h) - clipBo; d > 0 {
			sy1 -= d
		}
		if sx0 >= sx1 || sy0 >= sy1 {
			continue
		}

		col := cols[i]
		sa := col.A
		if sa <= 0 {
			continue
		}
		// Premultiplied source, matching the pixmap's storage.
		sr := uint32(clamp01(col.R*sa) * 255)
		sg := uint32(clamp01(col.G*sa) * 255)
		sb := uint32(clamp01(col.B*sa) * 255)
		saB := uint32(clamp01(sa) * 255)

		data := c.pixmap.Data()
		for sy := sy0; sy < sy1; sy++ {
			row := (y0+sy)*pw + (x0 + sx0)
			mrow := sy*stamp.w + sx0
			for sx := sx0; sx < sx1; sx++ {
				cov := uint32(stamp.cov[mrow])
				mrow++
				idx := row * 4
				row++
				if cov == 0 {
					continue
				}
				// Source-over with the stamp's coverage folded into the source
				// alpha: out = src*cov + dst*(1 - src.a*cov).
				a := saB * cov / 255
				inv := 255 - a
				data[idx+0] = uint8((sr*cov/255*255 + uint32(data[idx+0])*inv) / 255)
				data[idx+1] = uint8((sg*cov/255*255 + uint32(data[idx+1])*inv) / 255)
				data[idx+2] = uint8((sb*cov/255*255 + uint32(data[idx+2])*inv) / 255)
				data[idx+3] = uint8((a*255 + uint32(data[idx+3])*inv) / 255)
			}
		}
	}
	c.pixmap.NotifyPixelsChanged()
	return true
}

// rectClipBounds reports the current clip as a device-pixel rectangle, and
// whether it is rectangular at all. A non-rectangular clip returns false: its
// coverage varies per pixel and BlitMarks has nowhere to read that from.
func (c *Context) rectClipBounds() (left, top, right, bottom int, ok bool) {
	if c.clipStack == nil {
		return 0, 0, c.pixmap.Width(), c.pixmap.Height(), true
	}
	return clipRectOf(c.clipStack, c.pixmap.Width(), c.pixmap.Height())
}

// clipRectOf adapts the clip stack to a plain rectangle when it is one. A
// rounded or path clip returns false: its coverage varies per pixel, and this
// loop reads coverage only from the stamp.
func clipRectOf(cs *clip.ClipStack, w, h int) (left, top, right, bottom int, ok bool) {
	if cs.Depth() == 0 {
		return 0, 0, w, h, true
	}
	if !cs.IsRectOnly() {
		return 0, 0, 0, 0, false
	}
	r := cs.Bounds()
	return int(math.Floor(r.X)), int(math.Floor(r.Y)),
		int(math.Ceil(r.X + r.W)), int(math.Ceil(r.Y + r.H)), true
}
