package main

import (
	"image"
	"image/color"
	"math"
)

// A synthetic camera and microphone, so the effect can be seen on a platform
// that has no live capture — every desktop target today — and so the gallery
// thumbnail shows the mirror rather than an apology. It is always labelled on
// screen: a demo that quietly showed you a drawing when you asked for a camera
// would be worse than one that showed you nothing.

// syntheticSource renders a face and a sweeping spectrum, both animated by a
// frame counter, so it needs no clock and stays deterministic.
type syntheticSource struct {
	n     int
	pool  [2]*image.RGBA
	level float32
}

func newSynthetic() *syntheticSource { return &syntheticSource{} }

func (s *syntheticSource) Frame() *image.RGBA {
	s.n++
	i := s.n % len(s.pool)
	if s.pool[i] == nil {
		s.pool[i] = drawFace(640, 480)
	}
	// The face itself is static; the warp is what moves. Rotating the pool keeps
	// the identity contract the scene relies on.
	return s.pool[i]
}

// Level swells and fades like speech, so the chroma split and the sway breathe.
//
// It never falls near silence, unlike a real voice. A stand-in that went quiet
// would leave the face still and unwarped for stretches at a time, and a
// screenshot or a gallery thumbnail is one arbitrary frame — it would sooner or
// later land on one of them and show a demo that appears to do nothing.
func (s *syntheticSource) Level() float32 {
	t := float64(s.n) / 60
	v := 0.75 + 0.20*math.Sin(t*2.1) + 0.05*math.Sin(t*6.7)
	s.level = float32(clampf(v, 0, 1))
	return s.level
}

// Bands is a formant-ish shape that slides across the spectrum.
func (s *syntheticSource) Bands(dst []float32) int {
	t := float64(s.n) / 60
	for i := range dst {
		f := float64(i) / float64(len(dst))
		// Two moving peaks, rolling off toward the top end like a real voice.
		a := math.Exp(-sq(f-0.18-0.06*math.Sin(t*1.3)) * 120)
		b := math.Exp(-sq(f-0.52-0.10*math.Sin(t*0.8+1)) * 60)
		dst[i] = float32(clampf((a+0.7*b)*(1-f*0.5)*float64(s.level), 0, 1))
	}
	return len(dst)
}

func sq(v float64) float64 { return v * v }

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// drawFace paints a stand-in portrait: a lit oval with eyes and a mouth, which
// is all the warp needs to have something recognisable to bend.
func drawFace(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)*0.54
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x) - cx) / (float64(w) * 0.23)
			dy := (float64(y) - cy) / (float64(h) * 0.36)
			d := math.Hypot(dx, dy)

			// Background: a soft vignette, so the frame isn't flat.
			vx := (float64(x)/float64(w) - 0.5) * 2
			vy := (float64(y)/float64(h) - 0.5) * 2
			g := clampf(1-0.55*math.Hypot(vx, vy), 0, 1)
			c := color.RGBA{R: uint8(26 * g), G: uint8(34 * g), B: uint8(54 * g), A: 255}

			if d < 1 {
				k := clampf(1-d*0.5, 0, 1) * clampf(1.05-0.35*dy, 0, 1.2)
				c = color.RGBA{R: sat(232 * k), G: sat(180 * k), B: sat(150 * k), A: 255}
				for _, e := range [][3]float64{{-0.34, -0.20, 0.11}, {0.34, -0.20, 0.11}} {
					if math.Hypot(dx-e[0], (dy-e[1])*1.6) < e[2] {
						c = color.RGBA{R: 38, G: 30, B: 34, A: 255}
					}
				}
				if math.Hypot(dx, (dy-0.36)*1.5) < 0.24 {
					c = color.RGBA{R: 96, G: 42, B: 48, A: 255}
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func sat(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
