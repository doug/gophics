// Command flowfield is the flow-field generative-art staple (Processing, Nature
// of Code): a scalar field is read as an angle at every point, and particles are
// pushed along it, tracing the invisible field as braided streamlines. The field
// here is a sum of sines (no noise tables) that slowly evolves; each particle
// keeps a short trail, so the flow draws itself. All inside one widget.Canvas.
//
//	go run ./examples/flowfield
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
	numParticles = 700
	trailLen     = 15
	speed        = 1.8
)

type particle struct{ trail []geom.Pt }

type Flow struct{}

func (Flow) CreateState() widget.State { return &flowState{} }

type flowState struct {
	widget.StateBase[Flow]
	ps      []*particle
	w, h    float32
	t       float32
	started bool
}

func (s *flowState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

func (s *flowState) seed(w, h float32) {
	s.w, s.h = w, h
	s.ps = s.ps[:0]
	for range numParticles {
		p := geom.Pt{X: rand.Float32() * w, Y: rand.Float32() * h}
		s.ps = append(s.ps, &particle{trail: []geom.Pt{p}})
	}
	s.started = true
}

// angleAt reads the field. Summed sines at different scales make the streamlines
// fold back on themselves and braid instead of all flowing one way.
func (s *flowState) angleAt(p geom.Pt) float64 {
	x, y, t := float64(p.X), float64(p.Y), float64(s.t)
	v := math.Sin(x*0.006+t*0.30) + math.Cos(y*0.006-t*0.20) + math.Sin((x+y)*0.003+t*0.15)
	return v * 1.5
}

func (s *flowState) Tick(dt float64) bool {
	if !s.started {
		return true
	}
	s.SetState(func() { s.step(float32(dt)) })
	return true
}

func (s *flowState) step(dt float32) {
	s.t += dt
	for _, p := range s.ps {
		head := p.trail[len(p.trail)-1]
		next := head.Add(fromAngle(s.angleAt(head), speed))
		if next.X < 0 || next.X > s.w || next.Y < 0 || next.Y > s.h {
			// Re-seed off-screen particles so the field stays populated.
			next = geom.Pt{X: rand.Float32() * s.w, Y: rand.Float32() * s.h}
			p.trail = p.trail[:0]
		}
		p.trail = append(p.trail, next)
		if len(p.trail) > trailLen {
			p.trail = p.trail[len(p.trail)-trailLen:]
		}
	}
}

var bg = paint.RGB(0.02, 0.024, 0.047)

func (s *flowState) Build(widget.Ctx) widget.Widget {
	return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		if !s.started || s.w != size.W || s.h != size.H {
			s.seed(size.W, size.H)
		}
		c.Clear(bg)
		for _, p := range s.ps {
			if len(p.trail) < 2 {
				continue
			}
			head := p.trail[len(p.trail)-1]
			hue := wrapf(float32(s.angleAt(head)*180/math.Pi)*0.5+200, 0, 360)
			path := paint.NewPath()
			path.MoveTo(p.trail[0])
			for _, pt := range p.trail[1:] {
				path.LineTo(pt)
			}
			c.StrokePath(path, 1.3, paint.HSV(hue, 0.6, 1).WithAlpha(0.5))
		}
	}}
}

func fromAngle(a float64, l float32) geom.Pt {
	return geom.Pt{X: l * float32(math.Cos(a)), Y: l * float32(math.Sin(a))}
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
	if err := app.Run(Flow{}, app.Config{
		Title:      "gophics flow field",
		Size:       geom.Size{W: 900, H: 640},
		Background: bg,
	}); err != nil {
		log.Fatal(err)
	}
}
