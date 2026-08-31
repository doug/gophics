package chart

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"golang.org/x/image/font/gofont/goregular"
)

// Measures where a scatter stops being interactive today. PointMark issues one
// FillRRect per datum, so cost is linear in the point count with no batching —
// this is the number the "instanced marks" question is really about.
//
// Logged rather than asserted: it is a characterisation of the current ceiling,
// and a machine-dependent threshold would only ever be flaky.
func TestScatterScaleCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	for _, n := range []int{1_000, 10_000, 50_000, 100_000} {
		data := make([]Datum, n)
		for i := range data {
			f := float64(i)
			data[i] = Datum{X: f, Y: math.Sin(f/97) * 50}
		}
		h, err := app.NewHeadless(
			Chart{Marks: []Mark{PointMark{Data: data, Size: 3}}},
			app.Config{
				Size: geom.Size{W: 800, H: 500}, Background: paint.RGB(1, 1, 1),
				Font: goregular.TTF,
			}, 1)
		if err != nil {
			t.Fatal(err)
		}
		for range 30 { // settle the mount animation
			h.Step(0.05)
		}
		h.Render()

		const frames = 5
		start := time.Now()
		for range frames {
			h.Step(0.016)
			h.Render()
		}
		per := float64(time.Since(start).Microseconds()) / frames / 1000
		t.Logf("%7d points → %8.1f ms/frame  %s", n, per, verdict(per))
	}
}

func verdict(ms float64) string {
	switch {
	case ms <= 16.7:
		return "60fps"
	case ms <= 33.3:
		return "30fps"
	default:
		return fmt.Sprintf("~%.0ffps", 1000/ms)
	}
}
