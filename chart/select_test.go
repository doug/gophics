package chart

import (
	"strings"
	"testing"
	"time"
)

// The selection tooltip names the datum the way the x axis would: a time
// series reads as a date, not as the epoch seconds behind it, and a caller's
// XAxis.Format is honoured. It used to print fmtNumber(d.X) regardless, so a
// tap on any time series said something like "1,758.9M".
func TestSelectionLabelFollowsXAxis(t *testing.T) {
	lo := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ts := NewTime(lo, lo.AddDate(0, 0, 30))
	at := lo.AddDate(0, 0, 9)

	if got := selectionLabel(ts, Axis{}, Datum{X: Seconds(at), Y: 1}); got != "Mar 10" {
		t.Fatalf("time-series selection label = %q, want the date", got)
	}
	long := NewTime(lo, lo.AddDate(3, 0, 0))
	if got := selectionLabel(long, Axis{}, Datum{X: Seconds(at)}); !strings.Contains(got, "2026") {
		t.Fatalf("multi-year selection label %q omits the year", got)
	}
	short := NewTime(lo, lo.Add(6*time.Hour))
	if got := selectionLabel(short, Axis{}, Datum{X: Seconds(lo.Add(90 * time.Minute))}); !strings.Contains(got, "01:30") {
		t.Fatalf("intraday selection label %q omits the clock time", got)
	}

	fmtd := Axis{Format: func(v float64) string { return "week " + trim(v, Axis{}.loc) }}
	if got := selectionLabel(NewLinear(0, 10), fmtd, Datum{X: 3}); got != "week 3" {
		t.Fatalf("XAxis.Format ignored: got %q", got)
	}
	if got := selectionLabel(ts, fmtd, Datum{X: Seconds(at)}); !strings.HasPrefix(got, "week ") {
		t.Fatalf("XAxis.Format ignored on a time scale: got %q", got)
	}
	if got := selectionLabel(ts, fmtd, Datum{X: 1, Label: "Q1"}); got != "Q1" {
		t.Fatalf("a category label lost to the formatter: got %q", got)
	}
	if got := selectionLabel(NewLinear(0, 10), Axis{}, Datum{X: 2500}); got != "2.5k" {
		t.Fatalf("numeric selection label = %q, want the compact number", got)
	}
}
