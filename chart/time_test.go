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

// A run of ticks starts from the calendar boundary at or before Lo, which is
// before Lo whenever Lo is not itself on the boundary. Such a tick has a
// negative position: its gridline landed left of the plot and its label was
// clamped into the y-axis column. No branch may emit one.
func TestTimeTicksNeverPrecedeLo(t *testing.T) {
	cases := []struct {
		name   string
		lo, hi time.Time
	}{
		{"daily from noon", time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC), time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)},
		{"two-daily from noon", time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC), time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)},
		{"weekly from Monday afternoon", time.Date(2026, 1, 5, 15, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}, // Jan 5 2026 is a Monday
		{"monthly from mid-month", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"yearly from mid-year", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ticks := NewTime(c.lo, c.hi).Ticks(0)
			if len(ticks) == 0 {
				t.Fatal("no ticks")
			}
			for _, tk := range ticks {
				if tk.Pos < 0 || tk.Pos > 1 {
					t.Fatalf("tick %q at %v is outside the scale", tk.Label, tk.Pos)
				}
			}
		})
	}
}
