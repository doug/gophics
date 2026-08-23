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

// syntheticSource renders a cartoon and a sweeping spectrum, both animated by a
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
		s.pool[i] = drawFriend(640, 480)
	}
	// The drawing itself is static; the warp is what moves. Rotating the pool keeps
	// the identity contract the scene relies on.
	return s.pool[i]
}

// Level swells and fades like speech, so the chroma split and the sway breathe.
//
// It never falls near silence, unlike a real voice. A stand-in that went quiet
// would leave the drawing still and unwarped for stretches at a time, and a
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

// drawFriend paints the stand-in: a flat cartoon blob with big eyes, on a
// teal ground.
//
// It replaces a lit oval with skin tones, eye sockets and a dark mouth, which
// was an attempt at a portrait and landed squarely in the uncanny valley —
// unsettling on its own and worse once the warp started bending it. Nothing
// here is trying to be a person: saturated flat colour, no shading, features
// that read as drawn. A cartoon that warps is funny; a face that warps is not.
//
// The shapes stay bold and high-contrast because the warp needs edges to bend.
// A subtle image would ripple invisibly and the demo would look broken.
func drawFriend(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fw, fh := float64(w), float64(h)
	cx, cy := fw/2, fh*0.52
	r := math.Min(fw, fh) * 0.34 // body radius

	// Flat palette, deliberately not skin: cyan ground, warm yellow body.
	var (
		ground = [3]float64{18, 92, 104}
		body   = [3]float64{255, 206, 84}
		ink    = [3]float64{34, 40, 52}
		white  = [3]float64{255, 255, 255}
		blush  = [3]float64{247, 141, 122}
	)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)

			// Ground with a gentle vignette so the frame is not flat.
			vx, vy := (fx/fw-0.5)*2, (fy/fh-0.5)*2
			g := clampf(1-0.45*math.Hypot(vx, vy), 0, 1)
			c := [3]float64{ground[0] * g, ground[1] * g, ground[2] * g}

			// A couple of drifting dots, so an arbitrary frame still has
			// something off-centre in it.
			for _, d := range [][3]float64{{0.16, 0.20, 0.035}, {0.86, 0.30, 0.025}, {0.78, 0.82, 0.03}} {
				if math.Hypot(fx-d[0]*fw, fy-d[1]*fh) < d[2]*fw {
					c = [3]float64{ground[0] * 2.2, ground[1] * 1.3, ground[2] * 1.3}
				}
			}

			d := math.Hypot(fx-cx, (fy-cy)*1.06)
			if d < r {
				c = body

				// Big friendly eyes, set wide and high — cartoon proportions,
				// not anatomical ones.
				for _, e := range [][2]float64{{-0.36, -0.18}, {0.36, -0.18}} {
					ex, ey := cx+e[0]*r, cy+e[1]*r
					switch de := math.Hypot(fx-ex, fy-ey); {
					case de < r*0.20:
						c = white
					case de < r*0.235:
						c = ink
					}
					// Pupil, offset slightly so it looks alive rather than blank.
					if math.Hypot(fx-ex-r*0.03, fy-ey+r*0.02) < r*0.095 {
						c = ink
					}
				}

				// Rosy cheeks.
				for _, k := range [][2]float64{{-0.52, 0.18}, {0.52, 0.18}} {
					if math.Hypot(fx-(cx+k[0]*r), (fy-(cy+k[1]*r))*1.4) < r*0.13 {
						c = [3]float64{
							body[0]*0.35 + blush[0]*0.65,
							body[1]*0.35 + blush[1]*0.65,
							body[2]*0.35 + blush[2]*0.65,
						}
					}
				}

				// A smile: the lower arc of a ring, so it curves.
				sr := math.Hypot((fx-cx)/(r*0.46), (fy-cy-r*0.10)/(r*0.42))
				if fy > cy+r*0.12 && sr > 0.78 && sr < 1.0 {
					c = ink
				}

			}

			// The antenna sits above the body, so it is drawn outside the body
			// test — inside it, the bobble fell beyond the radius and never
			// appeared while the stalk only notched the top edge.
			if math.Abs(fx-cx) < r*0.022 && fy < cy-r*0.99 && fy > cy-r*1.20 {
				c = ink
			}
			if math.Hypot(fx-cx, fy-(cy-r*1.24)) < r*0.075 {
				c = blush
			}
			img.SetRGBA(x, y, color.RGBA{R: sat(c[0]), G: sat(c[1]), B: sat(c[2]), A: 255})
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
