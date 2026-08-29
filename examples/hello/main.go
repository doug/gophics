//go:build !js

// Command hello is the minimal gophics app: a colored, vsynced, resizable
// window driven through the shell + paint layers (no widgets).
package main

import (
	"log"
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/desktop"
)

type app struct {
	painter *paint.Painter
	t       float64
}

func (a *app) Frame(w shell.Window, f shell.Frame, dt float64) {
	a.t += dt
	p := float32(0.5 + 0.5*math.Sin(a.t*2))
	c := a.painter.BeginOffscreen(f.Size(), f.Scale())
	c.Clear(paint.Lerp(paint.RGB(0.07, 0.08, 0.16), paint.RGB(0.16, 0.10, 0.24), p))
	// Present the finished surface to the shell's frame target — the seam this
	// example exists to show. The painter is platform-agnostic; the caller
	// routes it to whichever target the frame offers (in real apps the app
	// runtime does this dance for you — see app.present).
	switch t := f.Target().(type) {
	case shell.PixelTarget:
		if s := a.painter.SurfaceRGBA(); s != nil {
			// The whole surface: this example paints every pixel each frame.
			t.Put(s, geom.RectFromSize(f.Size()))
		}
	}
	w.Invalidate() // continuous animation: request the next frame
}

func (a *app) Event(w shell.Window, e shell.Event) {
	switch e := e.(type) {
	case shell.Resize:
		log.Printf("resize: %.0fx%.0f @%gx", e.Size.W, e.Size.H, e.Scale)
	case shell.Closed:
		log.Println("closed")
	}
}

func main() {
	err := desktop.Run(&app{painter: paint.NewPainter()}, shell.Config{
		Title:     "gophics hello",
		Size:      geom.Size{W: 800, H: 600},
		Resizable: true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
