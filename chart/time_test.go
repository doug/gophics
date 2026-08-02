package chart

import (
	"math"
	"testing"
	"time"
)

func TestTimeScaleMap(t *testing.T) {
	lo := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	hi := lo.AddDate(0, 0, 30)
	s := NewTime(lo, hi)
	if got := s.Map(Seconds(lo)); math.Abs(float64(got)) > 1e-6 {
		t.Fatalf("Map(lo) = %v, want 0", got)
	}
	if got := s.Map(Seconds(hi)); math.Abs(float64(got)-1) > 1e-6 {
		t.Fatalf("Map(hi) = %v, want 1", got)
	}
	mid := lo.AddDate(0, 0, 15)
	if got := s.Map(Seconds(mid)); got < 0.49 || got > 0.51 {
		t.Fatalf("Map(mid) = %v, want ~0.5", got)
	}
}

func TestTimeScaleWeeklyTicks(t *testing.T) {
	lo := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) // ~30-day span → weekly Mondays
	s := NewTime(lo, lo.AddDate(0, 0, 29))
	ticks := s.Ticks(0)
	if len(ticks) < 3 {
		t.Fatalf("got %d ticks, want ≥3", len(ticks))
	}
	for _, tk := range ticks {
		d := time.Unix(int64(tk.Value), 0).UTC()
		if d.Weekday() != time.Monday {
			t.Fatalf("tick %v is not a Monday", d.Format("Jan 2"))
		}
		if tk.Pos < 0 || tk.Pos > 1 {
			t.Fatalf("tick pos %v out of range", tk.Pos)
		}
	}
}

func TestTimeScaleMonthlyTicks(t *testing.T) {
	lo := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) // ~6-month span → monthly
	s := NewTime(lo, lo.AddDate(0, 6, 0))
	for _, tk := range s.Ticks(0) {
		if d := time.Unix(int64(tk.Value), 0).UTC(); d.Day() != 1 {
			t.Fatalf("monthly tick %v is not the 1st", d.Format("Jan 2"))
		}
	}
}
