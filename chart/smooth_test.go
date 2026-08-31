package chart

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A smoothed line must not draw a value the data does not contain.
//
// Catmull-Rom, which this used to use, bulged 5.9 units past a data maximum of
// 100 on a flat-spike-flat series — the shape a live chart takes when a new
// reading arrives, and the reason an updating line looked jarring. Overshoot
// on a plot is not a cosmetic wobble: it is the chart asserting a value that
// was never measured.
func TestSmoothCurveNeverOvershootsTheData(t *testing.T) {
	cases := map[string][]geom.Pt{
		"spike up":     {{X: 0, Y: 100}, {X: 1, Y: 100}, {X: 2, Y: 20}, {X: 3, Y: 100}, {X: 4, Y: 100}},
		"spike down":   {{X: 0, Y: 20}, {X: 1, Y: 20}, {X: 2, Y: 100}, {X: 3, Y: 20}, {X: 4, Y: 20}},
		"step":         {{X: 0, Y: 10}, {X: 1, Y: 10}, {X: 2, Y: 90}, {X: 3, Y: 90}},
		"sawtooth":     {{X: 0, Y: 0}, {X: 1, Y: 50}, {X: 2, Y: 0}, {X: 3, Y: 50}, {X: 4, Y: 0}},
		"monotone up":  {{X: 0, Y: 0}, {X: 1, Y: 5}, {X: 2, Y: 40}, {X: 3, Y: 41}, {X: 4, Y: 90}},
		"flat then up": {{X: 0, Y: 30}, {X: 1, Y: 30}, {X: 2, Y: 30}, {X: 3, Y: 70}},
		"noisy":        {{X: 0, Y: 12}, {X: 1, Y: 88}, {X: 2, Y: 30}, {X: 3, Y: 91}, {X: 4, Y: 7}, {X: 5, Y: 55}},
	}
	for name, pts := range cases {
		t.Run(name, func(t *testing.T) {
			lo, hi := pts[0].Y, pts[0].Y
			for _, p := range pts {
				lo, hi = min(lo, p.Y), max(hi, p.Y)
			}
			b := smoothPath(pts).Bounds()
			const eps = 0.01
			if b.Min.Y < lo-eps || b.Max.Y > hi+eps {
				t.Errorf("curve spans %.2f..%.2f but the data spans %.2f..%.2f — "+
					"the plot is drawing values that were never measured",
					b.Min.Y, b.Max.Y, lo, hi)
			}
		})
	}
}

// Out-of-order X has no monotone solution. Straight segments are the honest
// fallback; inventing a shape is not.
func TestSmoothFallsBackWhenXIsNotIncreasing(t *testing.T) {
	pts := []geom.Pt{{X: 0, Y: 0}, {X: 2, Y: 10}, {X: 1, Y: 20}}
	b := smoothPath(pts).Bounds()
	if b.Min.Y < 0 || b.Max.Y > 20 {
		t.Errorf("fallback path spans %.2f..%.2f, want it held to the points", b.Min.Y, b.Max.Y)
	}
}

// Duplicated X (two readings sharing a timestamp) must not divide by zero.
func TestSmoothHandlesDuplicateX(t *testing.T) {
	pts := []geom.Pt{{X: 0, Y: 0}, {X: 1, Y: 10}, {X: 1, Y: 20}, {X: 2, Y: 5}}
	if smoothPath(pts) == nil {
		t.Fatal("nil path")
	}
}
