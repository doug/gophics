package chart

import (
	"testing"
	"time"
)

// XTime makes an inferred X scale a Time rather than a Linear.
//
// NewLinear snaps its domain out to round bounds, which is right for a
// quantity and wrong for seconds since the epoch: the round numbers land on
// arbitrary instants, so the axis ran months past both ends of the data with
// labels for periods holding none. It cannot be detected — a Datum holds a
// float64 and seconds look like any other large number — so it is said.
func TestXTimeInfersATimeScale(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	data := []Datum{
		{X: Seconds(start), Y: 1},
		{X: Seconds(start.AddDate(0, 0, 30)), Y: 4},
	}
	marks := []Mark{LineMark{Data: data}}

	linear, _ := resolveScales(Chart{Marks: marks})
	if _, ok := linear.(*Time); ok {
		t.Fatal("without XTime the scale should stay Linear")
	}

	timed, _ := resolveScales(Chart{Marks: marks, XTime: true})
	ts, ok := timed.(*Time)
	if !ok {
		t.Fatalf("with XTime the scale is %T, want *Time", timed)
	}

	// And it spans the data rather than snapping past it: the first datum sits
	// at the left edge and the last at the right, which is what a Linear scale
	// rounding out to nice bounds does not do.
	if got := ts.Map(Seconds(start)); got != 0 {
		t.Errorf("first datum maps to %v, want the left edge 0", got)
	}
	if got := ts.Map(Seconds(start.AddDate(0, 0, 30))); got != 1 {
		t.Errorf("last datum maps to %v, want the right edge 1", got)
	}
}

// An explicit X wins: XTime is only about what to infer.
func TestExplicitXBeatsXTime(t *testing.T) {
	marks := []Mark{LineMark{Data: []Datum{{X: 0, Y: 1}, {X: 10, Y: 2}}}}
	xs, _ := resolveScales(Chart{Marks: marks, XTime: true, X: NewLinear(0, 10)})
	if _, ok := xs.(*Time); ok {
		t.Error("XTime overrode an explicit X scale")
	}
}
