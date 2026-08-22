//go:build gophics_gpu

package main

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

// The same scrolling frame through the GPU rasterizer, to see whether the cost
// measured on the CPU path is what a real user pays.
func BenchmarkTelemetryScrollGPU(b *testing.B) {
	store := NewStore(7)
	stop := make(chan struct{})
	go store.Produce(stop)
	time.Sleep(400 * time.Millisecond)
	close(stop)

	var st *dash
	stateHook = func(d *dash) { st = d }
	h, err := app.NewHeadless(App{Store: store}, app.Config{
		Size: geom.Size{W: 1500, H: 700}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		b.Fatal(err)
	}
	h.Render()
	stateHook = nil
	st.live = false

	if h.RenderGPU() == nil {
		b.Skip("no headless GPU adapter")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Scroll(geom.Pt{Y: -40})
		h.Step(1.0 / 60)
		h.RenderGPU()
	}
}
