// Command mover is a minimal real-time input demo: a square you drive with held
// WASD or arrow keys (and Space to dash), polling Ctx.Input() every frame. It is
// the smallest thing that proves the input workstream — held-key state driving
// motion at 60fps, focus-free.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

type Mover struct{}

func (Mover) CreateState() widget.State { return &moverState{} }

type moverState struct {
	widget.StateBase[Mover]
	ctx  widget.Ctx
	in   *input.State
	pos  geom.Pt
	size geom.Size
}

func (s *moverState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.in = ctx.Input()
	s.pos = geom.Pt{X: 300, Y: 220}
	s.size = geom.Size{W: 600, H: 440}
	ctx.AddTicker(moverTick{s})
}

type moverTick struct{ s *moverState }

func (t moverTick) Tick(dt float64) bool {
	s := t.s
	if s.in == nil {
		return true
	}
	dx := clamp1(s.in.Axis(shell.KeyA, shell.KeyD) + s.in.Axis(shell.KeyLeft, shell.KeyRight))
	dy := clamp1(s.in.Axis(shell.KeyW, shell.KeyS) + s.in.Axis(shell.KeyUp, shell.KeyDown))
	speed := float32(320)
	if s.in.Down(shell.KeySpace) {
		speed = 680 // dash
	}
	s.pos.X += dx * speed * float32(dt)
	s.pos.Y += dy * speed * float32(dt)
	s.pos.X = clampf(s.pos.X, 20, s.size.W-20)
	s.pos.Y = clampf(s.pos.Y, 20, s.size.H-20)
	s.ctx.Invalidate() // the position moved this tick — request a repaint
	return true        // stay active: keep ticking every frame to poll held keys
}

func (s *moverState) Build(ctx widget.Ctx) widget.Widget {
	return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		s.size = size
		c.Clear(paint.RGB(0.08, 0.09, 0.12))
		c.FillRRect(geom.RectXYWH(s.pos.X-18, s.pos.Y-18, 36, 36), 9, paint.RGB(0.42, 0.72, 0.96))
		c.TextIn("", "WASD / arrows to move · hold Space to dash",
			geom.Pt{X: 16, Y: 28}, 15, paint.RGB(0.7, 0.73, 0.8))

	}}
}

func clamp1(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func clampf(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	err := app.Run(Mover{}, app.Config{
		Title: "Mover", Size: geom.Size{W: 600, H: 440},
		Font: goregular.TTF,
	})
	if err != nil {
		log.Fatal(err)
	}
}
