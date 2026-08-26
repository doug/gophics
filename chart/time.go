package chart

import "time"

// Time is a continuous scale over a date range whose ticks fall on calendar
// boundaries (days, weeks, months, or years depending on span). Datum X values
// are Unix seconds; use Seconds(t) to convert.
type Time struct {
	Lo, Hi time.Time
}

// NewTime builds a time scale spanning [lo, hi].
func NewTime(lo, hi time.Time) *Time { return &Time{Lo: lo, Hi: hi} }

// Seconds is the Datum X value for a time instant.
//
// Set Chart.XTime alongside it, or pass an explicit Chart.X of NewTime. A
// Datum holds a float64 and seconds are indistinguishable from any other large
// number, so without one of those the chart infers a Linear scale and the axis
// runs well past the data.
func Seconds(t time.Time) float64 { return float64(t.Unix()) }

func (s *Time) lo() float64 { return float64(s.Lo.Unix()) }
func (s *Time) hi() float64 { return float64(s.Hi.Unix()) }

func (s *Time) Map(v float64) float32 {
	lo, hi := s.lo(), s.hi()
	if hi == lo {
		return 0
	}
	return float32((v - lo) / (hi - lo))
}

func (s *Time) Invert(t float32) float64   { return s.lo() + float64(t)*(s.hi()-s.lo()) }
func (s *Time) Domain() (float64, float64) { return s.lo(), s.hi() }

func (s *Time) tick(d time.Time, layout string) Tick {
	return Tick{Value: Seconds(d), Pos: s.Map(Seconds(d)), Label: d.Format(layout)}
}

// Ticks chooses calendar-aligned ticks by span: days → daily, up to a quarter →
// weekly (Mondays), up to two years → monthly, else yearly.
func (s *Time) Ticks(_ int) []Tick {
	days := s.Hi.Sub(s.Lo).Hours() / 24
	var out []Tick
	switch {
	case days <= 14:
		step := 1
		if days > 8 {
			step = 2
		}
		for d := dayStart(s.Lo); !d.After(s.Hi); d = d.AddDate(0, 0, step) {
			out = append(out, s.tick(d, "Jan 2"))
		}
	case days <= 92:
		for d := nextMonday(s.Lo); !d.After(s.Hi); d = d.AddDate(0, 0, 7) {
			out = append(out, s.tick(d, "Jan 2"))
		}
	case days <= 730:
		for d := monthStart(s.Lo); !d.After(s.Hi); d = d.AddDate(0, 1, 0) {
			if !d.Before(s.Lo) {
				out = append(out, s.tick(d, "Jan"))
			}
		}
	default:
		for d := yearStart(s.Lo); !d.After(s.Hi); d = d.AddDate(1, 0, 0) {
			if !d.Before(s.Lo) {
				out = append(out, s.tick(d, "2006"))
			}
		}
	}
	return out
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func yearStart(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

// nextMonday returns the first Monday at or after t (day-aligned).
func nextMonday(t time.Time) time.Time {
	d := dayStart(t)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
