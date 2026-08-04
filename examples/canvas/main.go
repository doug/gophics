// Command canvas is a generative-art demo of the widget.Canvas escape hatch.
//
// It draws an interference field of pulsing dots — hundreds of fills plus live
// text, re-recorded every frame — entirely inside one Canvas.Draw callback, in
// local coordinates. It exercises the custom-paint surface end to end through
// whichever rasterizer is resolved at runtime — GPU by default, CPU as a
// fallback or when forced (the on-screen readout says which).
//
//	go run ./examples/canvas                        # GPU (default), CPU if none
//	GOPHICS_RENDERER=cpu go run ./examples/canvas  # force CPU
package main

import (
	"fmt"
	"log"
	"math"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Field is a stateful widget that advances a clock every frame and rebuilds a
// full-window Canvas from it.
type Field struct{}

func (Field) CreateState() widget.State { return &fieldState{} }

type fieldState struct {
	widget.StateBase[Field]
	t float64 // seconds since start
}

// Init registers this state as a per-frame ticker.
func (s *fieldState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

// Tick advances the clock and repaints; returning true keeps it animating.
func (s *fieldState) Tick(dt float64) bool {
	s.SetState(func() { s.t += dt })
	return true
}

var (
	colTop = paint.RGB(0.05, 0.06, 0.11)
	colBot = paint.RGB(0.01, 0.01, 0.03)
	colA   = paint.RGB(0.20, 0.85, 0.90) // cyan crest
	colB   = paint.RGB(0.90, 0.25, 0.72) // magenta trough
	colTxt = paint.RGB(0.92, 0.94, 0.97)
	colDim = paint.RGB(0.45, 0.50, 0.62)
)

func (s *fieldState) Build(widget.Ctx) widget.Widget {
	t := s.t
	return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		w, h := size.W, size.H

		// Background: a soft vertical gradient. Origin is (0,0) — local coords.
		c.FillRRectGradient(geom.Rect{Max: size.Pt()}, 0, colTop, colBot, false)

		// Two slow-orbiting wave sources drive an interference pattern.
		s1 := geom.Pt{
			X: w*0.5 + w*0.30*float32(math.Cos(t*0.70)),
			Y: h*0.5 + h*0.30*float32(math.Sin(t*0.90)),
		}
		s2 := geom.Pt{
			X: w*0.5 + w*0.34*float32(math.Cos(t*1.10+2)),
			Y: h*0.5 + h*0.28*float32(math.Sin(t*0.60+1)),
		}

		// A grid of dots whose size, color and opacity follow the summed waves.
		const step = 26
		for gy := float32(step) / 2; gy < h; gy += step {
			for gx := float32(step) / 2; gx < w; gx += step {
				d1 := math.Hypot(float64(gx-s1.X), float64(gy-s1.Y))
				d2 := math.Hypot(float64(gx-s2.X), float64(gy-s2.Y))
				wave := (math.Sin(d1*0.050-t*3.0) + math.Sin(d2*0.045-t*2.3)) * 0.5
				m := float32(wave*0.5 + 0.5) // 0 (trough) .. 1 (crest)
				rad := 1.5 + m*6
				col := paint.Lerp(colB, colA, m).WithAlpha(0.20 + 0.80*m)
				c.FillRRect(geom.RectXYWH(gx-rad, gy-rad, rad*2, rad*2), rad, col)
			}
		}

		// Title and a live readout, drawn into the same surface — this is the
		// text path that renders correctly on the GPU build.
		c.Text("gophics · canvas", geom.Pt{X: 28, Y: 50}, 30, colTxt)
		c.Text(fmt.Sprintf("generative interference field — %.1fs — %s rasterizer", t, rasterizer()),
			geom.Pt{X: 28, Y: 76}, 14, colDim)
	}}
}

func main() {
	if err := app.Run(Field{}, app.Config{
		Title:      "gophics canvas",
		Size:       geom.Size{W: 960, H: 620},
		Background: colBot,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
