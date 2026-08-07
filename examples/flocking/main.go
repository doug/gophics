// Command flocking is Craig Reynolds' boids — the flocking sketch from
// Processing's examples and Nature of Code. Each boid steers by three local
// rules against its neighbours: separation (avoid crowding), alignment (match
// heading), cohesion (seek the group centre). Nothing coordinates the flock;
// the structure is entirely emergent. Ported to the widget.Canvas escape hatch.
//
//	go run ./examples/flocking
package main

import (
	"log"
	"math"
	"math/rand"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

const (
	numBoids  = 200
	maxSpeed  = 2.6
	maxForce  = 0.05
	sepRadius = 26
	nbrRadius = 56
	sepWeight = 1.6
	aliWeight = 1.0
	cohWeight = 0.9
)

// --- tiny vector helpers over geom.Pt (self-contained, no framework additions) ---

func vlen(p geom.Pt) float32 { return float32(math.Hypot(float64(p.X), float64(p.Y))) }
func vnorm(p geom.Pt) geom.Pt {
	l := vlen(p)
	if l == 0 {
		return geom.Pt{}
	}
	return p.Mul(1 / l)
}
func vsetlen(p geom.Pt, l float32) geom.Pt { return vnorm(p).Mul(l) }
func vlimit(p geom.Pt, max float32) geom.Pt {
	if vlen(p) > max {
		return vsetlen(p, max)
	}
	return p
}
func vdist(a, b geom.Pt) float32 { return vlen(a.Sub(b)) }
func vangle(p geom.Pt) float64   { return math.Atan2(float64(p.Y), float64(p.X)) }
func fromAngle(a float64, l float32) geom.Pt {
	return geom.Pt{X: l * float32(math.Cos(a)), Y: l * float32(math.Sin(a))}
}

type boid struct{ pos, vel, acc geom.Pt }

type Flock struct{}

func (Flock) CreateState() widget.State { return &flockState{} }

type flockState struct {
	widget.StateBase[Flock]
	boids   []*boid
	w, h    float32
	started bool
}

func (s *flockState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

func (s *flockState) seed(w, h float32) {
	s.w, s.h = w, h
	s.boids = s.boids[:0]
	for i := 0; i < numBoids; i++ {
		s.boids = append(s.boids, &boid{
			pos: geom.Pt{X: rand.Float32() * w, Y: rand.Float32() * h},
			vel: fromAngle(rand.Float64()*2*math.Pi, 1+rand.Float32()*(maxSpeed-1)),
		})
	}
	s.started = true
}

// steer nudges vel toward a desired direction, capped so boids turn smoothly.
func steer(desired, vel geom.Pt) geom.Pt {
	if desired == (geom.Pt{}) {
		return geom.Pt{}
	}
	return vlimit(vsetlen(desired, maxSpeed).Sub(vel), maxForce)
}

func (s *flockState) Tick(dt float64) bool {
	if !s.started {
		return true
	}
	s.SetState(func() { s.step() })
	return true
}

func (s *flockState) step() {
	for _, b := range s.boids {
		var sep, ali, coh geom.Pt
		var nSep, nNbr float32
		for _, o := range s.boids {
			if o == b {
				continue
			}
			d := vdist(b.pos, o.pos)
			if d > 0 && d < sepRadius {
				sep = sep.Add(vnorm(b.pos.Sub(o.pos)).Mul(1 / d))
				nSep++
			}
			if d > 0 && d < nbrRadius {
				ali = ali.Add(o.vel)
				coh = coh.Add(o.pos)
				nNbr++
			}
		}
		if nSep > 0 {
			b.acc = b.acc.Add(steer(sep.Mul(1/nSep), b.vel).Mul(sepWeight))
		}
		if nNbr > 0 {
			b.acc = b.acc.Add(steer(ali.Mul(1/nNbr), b.vel).Mul(aliWeight))
			b.acc = b.acc.Add(steer(coh.Mul(1/nNbr).Sub(b.pos), b.vel).Mul(cohWeight))
		}
	}
	for _, b := range s.boids {
		b.vel = vlimit(b.vel.Add(b.acc), maxSpeed)
		b.pos = wrap(b.pos.Add(b.vel), s.w, s.h)
		b.acc = geom.Pt{}
	}
}

func wrap(p geom.Pt, w, h float32) geom.Pt {
	if p.X < 0 {
		p.X += w
	} else if p.X > w {
		p.X -= w
	}
	if p.Y < 0 {
		p.Y += h
	} else if p.Y > h {
		p.Y -= h
	}
	return p
}

var bg = paint.RGB(0.043, 0.055, 0.086)

func (s *flockState) Build(widget.Ctx) widget.Widget {
	return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		if !s.started || s.w != size.W || s.h != size.H {
			s.seed(size.W, size.H)
		}
		c.Clear(bg)
		for _, b := range s.boids {
			a := vangle(b.vel)
			col := paint.HSV(wrapf(float32(a*180/math.Pi), 0, 360), 0.55, 1)
			// an arrowhead pointing along the velocity
			p := paint.NewPath()
			p.MoveTo(tri(b.pos, a, 7, 0)).LineTo(tri(b.pos, a, -4, 3)).LineTo(tri(b.pos, a, -4, -3))
			c.FillPath(p.Close(), col)
		}
	}}
}

// tri rotates a local (lx,ly) by angle a and offsets it to pos.
func tri(pos geom.Pt, a float64, lx, ly float32) geom.Pt {
	ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
	return geom.Pt{X: pos.X + lx*ca - ly*sa, Y: pos.Y + lx*sa + ly*ca}
}

func wrapf(v, lo, hi float32) float32 {
	r := hi - lo
	for v < lo {
		v += r
	}
	for v >= hi {
		v -= r
	}
	return v
}

// hsv converts H∈[0,360), S,V∈[0,1] to a paint.Color.

func main() {
	if err := app.Run(Flock{}, app.Config{
		Title:      "gophics flocking",
		Size:       geom.Size{W: 900, H: 640},
		Background: bg,
	}); err != nil {
		log.Fatal(err)
	}
}
