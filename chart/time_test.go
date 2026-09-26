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
		{"hourly from half past", time.Date(2026, 1, 5, 9, 30, 0, 0, time.UTC), time.Date(2026, 1, 5, 15, 0, 0, 0, time.UTC)},
		{"six-hourly from 09:00", time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), time.Date(2026, 1, 6, 21, 0, 0, 0, time.UTC)},
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

// An intraday span gets hourly ticks. There used to be no branch finer than
// daily, so a chart of a single trading session had one midnight tick when it
// started at midnight and no axis at all otherwise, while its tooltip already
// read in clock time. The step follows the span (1, 2, 3, or 6 hours), the run
// is aligned to the local day, and the clock labels carry the date on the
// first tick and whenever the run crosses into a new day.
func TestTimeScaleHourlyTicks(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800) // a half-hour offset, where Truncate on the instant lands off the hour
	cases := []struct {
		name   string
		lo     time.Time
		span   time.Duration
		step   time.Duration
		first  string
		nDated int // labels carrying the date
	}{
		{"six hours from 09:00", time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), 6 * time.Hour, time.Hour, "Jan 6 09:00", 1},
		{"twelve hours from midnight", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), 12 * time.Hour, 2 * time.Hour, "Jan 6 00:00", 1},
		{"twenty-three hours from 09:00", time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), 23 * time.Hour, 6 * time.Hour, "Jan 6 12:00", 2},
		{"thirty-six hours from 09:00", time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), 36 * time.Hour, 6 * time.Hour, "Jan 6 12:00", 2},
		{"two days from midnight", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), 48 * time.Hour, 6 * time.Hour, "Jan 6 00:00", 3},
		{"six hours from 09:30 in a half-hour zone", time.Date(2026, 1, 6, 9, 30, 0, 0, ist), 6 * time.Hour, time.Hour, "Jan 6 10:00", 1},
		{"a day from 09:00 in a half-hour zone", time.Date(2026, 1, 6, 9, 0, 0, 0, ist), 24 * time.Hour, 6 * time.Hour, "Jan 6 12:00", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewTime(c.lo, c.lo.Add(c.span))
			ticks := s.Ticks(0)
			if len(ticks) < 2 {
				t.Fatalf("got %d ticks %v, want an axis", len(ticks), labels(ticks))
			}
			if ticks[0].Label != c.first {
				t.Errorf("first label = %q, want %q", ticks[0].Label, c.first)
			}
			dated := 0
			var prev time.Time
			for i, tk := range ticks {
				d := time.Unix(int64(tk.Value), 0).In(c.lo.Location())
				if d.Before(c.lo) || d.After(s.Hi) || tk.Pos < 0 || tk.Pos > 1 {
					t.Errorf("tick %q at %v is outside the scale", tk.Label, d)
				}
				if d.Minute() != 0 || d.Second() != 0 || time.Duration(d.Hour())*time.Hour%c.step != 0 {
					t.Errorf("tick %v is not on a %v boundary of the local day", d, c.step)
				}
				if i > 0 && d.Sub(prev) != c.step {
					t.Errorf("tick %v follows %v; want a %v step", d, prev, c.step)
				}
				prev = d
				if len(tk.Label) > len("15:04") {
					dated++
					if tk.Label != d.Format("Jan 2 15:04") {
						t.Errorf("dated label %q, want %q", tk.Label, d.Format("Jan 2 15:04"))
					}
				} else if tk.Label != d.Format("15:04") {
					t.Errorf("label %q, want %q", tk.Label, d.Format("15:04"))
				}
			}
			if dated != c.nDated {
				t.Errorf("%d labels carry the date, want %d: %v", dated, c.nDated, labels(ticks))
			}
		})
	}
}

func labels(ticks []Tick) []string {
	out := make([]string, len(ticks))
	for i, t := range ticks {
		out[i] = t.Label
	}
	return out
}
