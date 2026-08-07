// Command particles is a fountain built from a simple particle system — a burst
// of coloured sparks that arc under gravity and fade out, re-recorded every
// frame inside one widget.Canvas.
//
//	go run ./examples/particles
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
	gravity  = 520 // px/s²
	drag     = 0.9 // per-second velocity retention
	perFrame = 14  // sparks emitted each frame
	maxAlive = 1400
)

type spark struct {
	pos, vel        geom.Pt
	life, max, size float32
	col             paint.Color
}

type Fountain struct{}

func (Fountain) CreateState() widget.State { return &fountainState{} }

type fountainState struct {
	widget.StateBase[Fountain]
	sparks []*spark
	w, h   float32
	t      float32
}

func (s *fountainState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

func (s *fountainState) Tick(dt float64) bool {
	s.SetState(func() { s.step(float32(dt)) })
	return true
}

func (s *fountainState) step(dt float32) {
	s.t += dt
	origin := geom.Pt{X: s.w * 0.5, Y: s.h * 0.92}
	// Emit an upward burst with a little spread.
	for i := 0; i < perFrame && len(s.sparks) < maxAlive; i++ {
		a := -math.Pi/2 + (rand.Float64()-0.5)*0.9
		sp := 260 + rand.Float32()*300
		hue := wrapf(s.t*70+rand.Float32()*50, 0, 360)
		s.sparks = append(s.sparks, &spark{
			pos: origin, vel: fromAngle(a, sp),
			life: 0.8 + rand.Float32()*1.3, max: 2.1, size: 2 + rand.Float32()*3,
			col: paint.HSV(hue, 0.7, 1),
		})
	}
	// Advance + cull.
	live := s.sparks[:0]
	dragF := float32(math.Pow(drag, float64(dt)))
	for _, p := range s.sparks {
		p.vel.Y += gravity * dt
		p.vel = p.vel.Mul(dragF)
		p.pos = p.pos.Add(p.vel.Mul(dt))
		p.life -= dt
		if p.life > 0 && p.pos.Y < s.h+20 {
			live = append(live, p)
		}
	}
	s.sparks = live
}

var bg = paint.RGB(0.02, 0.024, 0.043)

func (s *fountainState) Build(widget.Ctx) widget.Widget {
	return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		s.w, s.h = size.W, size.H
		c.FillRRectGradient(geom.Rect{Max: size.Pt()}, 0, paint.RGB(0.05, 0.06, 0.10), bg, false)
		for _, p := range s.sparks {
			r := p.size * (0.4 + 0.6*p.life/p.max)
			col := p.col.WithAlpha(clamp(p.life/p.max, 0, 1))
			c.FillRRect(geom.RectXYWH(p.pos.X-r, p.pos.Y-r, r*2, r*2), r, col)
		}
	}}
}

func fromAngle(a float64, l float32) geom.Pt {
	return geom.Pt{X: l * float32(math.Cos(a)), Y: l * float32(math.Sin(a))}
}
func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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

func main() {
	if err := app.Run(Fountain{}, app.Config{
		Title:      "gophics particles",
		Size:       geom.Size{W: 720, H: 560},
		Background: bg,
	}); err != nil {
		log.Fatal(err)
	}
}
