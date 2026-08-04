// Command customdraw is the SKILL.md custom-painting example: a *stateless*
// widget (Build only, no mutable state) that draws a small bar chart through
// widget.Canvas + paint.Canvas — the escape hatch for graphics that don't
// decompose into widgets. Compiled by CI, so the SKILL.md snippet stays honest.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

var (
	bg  = paint.RGB(0.09, 0.10, 0.13)
	bar = paint.RGB(0.36, 0.62, 0.98)
	ink = paint.RGB(0.92, 0.93, 0.95)
)

// Chart is stateless: it builds entirely from its fields, so it only implements
// Build (no CreateState, no State struct).
type Chart struct{ Values []float32 } // each value in [0,1]

func (c Chart) Build(ctx widget.Ctx) widget.Widget {
	return widget.Canvas{Clip: true, Draw: func(cv paint.Canvas, size geom.Size) {
		cv.Clear(bg)
		cv.Text("weekly", geom.Pt{X: 20, Y: 32}, 18, ink)
		n := len(c.Values)
		if n == 0 {
			return
		}
		const pad, gap = 20, 12
		w := (size.W - 2*pad - gap*float32(n-1)) / float32(n)
		for i, v := range c.Values {
			h := v * (size.H - 80)
			x := pad + float32(i)*(w+gap)
			cv.FillRRect(geom.RectXYWH(x, size.H-pad-h, w, h), 6, bar)
		}
	}}
}

func main() {
	root := Chart{Values: []float32{0.3, 0.7, 0.5, 0.9, 0.4, 0.8, 0.6}}
	if err := app.Run(root, app.Config{
		Title:      "gophics chart",
		Size:       geom.Size{W: 480, H: 320},
		Background: bg,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
