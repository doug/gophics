package main

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

// Frame cost while scrolling a full table. The table virtualizes, so this
// should be flat in the number of rows held.
func BenchmarkTelemetryScroll(b *testing.B) {
	store := NewStore(7)
	stop := make(chan struct{})
	go store.Produce(stop)
	time.Sleep(400 * time.Millisecond)
	close(stop)

	var st *dash
	stateHook = func(d *dash) { st = d }
	a := apptest.New(b, App{Store: store}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 1500, H: 700}, Font: goregular.TTF,
	}))
	a.Render()
	stateHook = nil
	st.live = false // isolate scrolling from the live tail

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Scroll(geom.Pt{Y: -40})
		a.Step(1.0 / 60)
		a.Render()
	}
	b.StopTimer()
	b.Logf("rows held: %d", len(st.res.View))
}
