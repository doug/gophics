// Command hello is the M0 exit-criterion demo: a colored, vsynced, resizable
// window driven through gossamer's shell interface.
package main

import (
	"log"
	"math"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/desktop"
)

type app struct {
	t float64
}

func (a *app) Frame(w shell.Window, f shell.Frame, dt float64) {
	a.t += dt
	// Pulse between two dusk-ish colors to make vsync pacing visible.
	p := float32(0.5 + 0.5*math.Sin(a.t*2))
	a.clearLerp(f, p)
	w.Invalidate() // continuous animation: request the next frame
}

func (a *app) clearLerp(f shell.Frame, p float32) {
	lerp := func(x, y float32) float32 { return x + (y-x)*p }
	f.Clear(lerp(0.07, 0.16), lerp(0.08, 0.10), lerp(0.16, 0.24), 1)
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
	err := desktop.Run(&app{}, shell.Config{
		Title:     "gossamer hello",
		Size:      geom.Size{W: 800, H: 600},
		Resizable: true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
