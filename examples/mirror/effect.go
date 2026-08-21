package main

import (
	"image"
	"math"
)

// The effect is deliberately a pure function over two pixel buffers and a few
// numbers: no camera, no microphone, no widgets. That is what makes it testable
// without hardware — the tests below drive it with a synthetic frame and
// synthetic audio — and it is also what keeps it fast, because the whole thing
// can be reduced to table lookups before the pixel loop starts.

// Params is one frame's worth of modulation.
type Params struct {
	// Level is the microphone's current level, 0..1.
	Level float32
	// Bands is the input spectrum, low frequency first, 0..1. Any length.
	Bands []float32
	// T is a monotonically rising time in seconds, for motion that continues
	// while the room is quiet.
	T float32
	// Amount scales the whole effect, 0..1 — the one control the UI exposes.
	Amount float32
	// Mirror flips horizontally. A front camera shows you un-mirrored, which
	// reads as somebody else's face rather than your own reflection.
	Mirror bool
}

// Warp writes a voice-modulated version of src into dst. dst and src must be
// the same size; Warp is a no-op otherwise rather than a panic, because the
// camera can change frame size underneath a running preview.
//
// The two displacements are separable — the horizontal one depends only on the
// row and the vertical one only on the column — so both collapse into lookup
// tables computed once per frame. The pixel loop is then four loads and a
// clamp, which is what lets this run per frame in Go rather than needing a
// shader.
func Warp(dst, src *image.RGBA, p Params) {
	b := src.Bounds()
	if dst.Bounds() != b || b.Empty() {
		return
	}
	w, h := b.Dx(), b.Dy()
	amt := clamp01(p.Amount)
	level := clamp01(p.Level)

	// Columns rise with the energy in their part of the spectrum, so the image
	// behaves like a bar chart made of your own face.
	colDY := make([]int, w)
	lift := amt * float32(h) * 0.10
	for x := 0; x < w; x++ {
		colDY[x] = int(bandAt(p.Bands, x, w) * lift)
	}

	// Rows slide sideways on a travelling wave whose amplitude is loudness, so
	// a quiet room is still and a shout ripples.
	rowDX := make([]int, h)
	sway := amt * level * float32(w) * 0.035
	for y := 0; y < h; y++ {
		rowDX[y] = int(sway * float32(math.Sin(float64(y)*0.055+float64(p.T)*3.2)))
	}

	// A colour split that opens up as you get louder: red and blue sampled a
	// few pixels either side of green.
	chroma := int(amt * level * float32(w) * 0.012)

	sp, dp := src.Pix, dst.Pix
	ss, ds := src.Stride, dst.Stride
	for y := 0; y < h; y++ {
		dx := rowDX[y]
		row := y * ds
		for x := 0; x < w; x++ {
			sx := x
			if p.Mirror {
				sx = w - 1 - x
			}
			sx += dx
			sy := y + colDY[x]

			g := clampi(sx, 0, w-1)
			gy := clampi(sy, 0, h-1)
			base := gy*ss + g*4

			o := row + x*4
			dp[o+1] = sp[base+1] // green stays put; it carries the luminance
			dp[o+3] = 255

			if chroma == 0 {
				dp[o] = sp[base]
				dp[o+2] = sp[base+2]
				continue
			}
			r := gy*ss + clampi(sx+chroma, 0, w-1)*4
			bl := gy*ss + clampi(sx-chroma, 0, w-1)*4
			dp[o] = sp[r]
			dp[o+2] = sp[bl+2]
		}
	}
}

// bandAt reads the spectrum at a column, interpolating between neighbouring
// bands and squaring the result.
//
// Both parts matter. Four dozen bands across a 640-pixel frame is one band
// every thirteen columns, so picking the nearest cuts the silhouette into a
// staircase; interpolating makes the lift continuous. And squaring keeps the
// noise floor near zero — without it every band sits slightly above silence and
// the whole image shimmers in an empty room.
func bandAt(bands []float32, x, w int) float32 {
	n := len(bands)
	if n == 0 || w <= 0 {
		return 0
	}
	p := (float32(x)+0.5)*float32(n)/float32(w) - 0.5
	i := int(math.Floor(float64(p)))
	f := p - float32(i)
	a := clamp01(bands[clampi(i, 0, n-1)])
	b := clamp01(bands[clampi(i+1, 0, n-1)])
	v := a + (b-a)*f
	return v * v
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
