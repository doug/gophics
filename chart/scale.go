// Package chart is a built-in, Swift Charts–style charting library: declarative
// marks (bars, lines, points, rules, heatmaps, ranges) composed over scales and
// rendered through paint.Canvas. It depends only on gossamer's own layers
// (paint, widget, geom, anim, layout) and the standard library.
//
// The smallest useful chart:
//
//	chart.Chart{Marks: []chart.Mark{
//	    chart.BarMark{Data: chart.Values("Mon", 3, "Tue", 5, "Wed", 2)},
//	}}
package chart

import (
	"math"
	"strconv"
)

// Tick is one axis gridline/label: a domain value, its 0..1 position along the
// axis, and an optional label (band scales set the category name; numeric scales
// leave it empty and let the axis format the value).
type Tick struct {
	Value float64
	Pos   float32
	Label string
}

// Scale maps domain values to a 0..1 position along an axis and back.
type Scale interface {
	// Map returns the 0..1 position of v (0 = axis start, 1 = axis end).
	Map(v float64) float32
	// Invert maps a 0..1 position back to a domain value.
	Invert(t float32) float64
	// Ticks returns up to ~target "nice" ticks across the domain.
	Ticks(target int) []Tick
	// Domain returns the numeric domain (lo, hi).
	Domain() (float64, float64)
}

// Linear is a continuous numeric scale over [Lo, Hi].
type Linear struct{ Lo, Hi float64 }

// NewLinear returns a Linear scale whose domain is snapped out to "nice" round
// bounds that comfortably contain [lo, hi].
func NewLinear(lo, hi float64) *Linear {
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi == lo { // give a zero-span domain some breathing room
		if hi == 0 {
			return &Linear{0, 1}
		}
		pad := math.Abs(hi) * 0.5
		return &Linear{lo - pad, hi + pad}
	}
	step := niceStep(lo, hi, 5)
	return &Linear{math.Floor(lo/step) * step, math.Ceil(hi/step) * step}
}

func (s *Linear) Map(v float64) float32 {
	if s.Hi == s.Lo {
		return 0
	}
	return float32((v - s.Lo) / (s.Hi - s.Lo))
}

func (s *Linear) Invert(t float32) float64   { return s.Lo + float64(t)*(s.Hi-s.Lo) }
func (s *Linear) Domain() (float64, float64) { return s.Lo, s.Hi }

func (s *Linear) Ticks(target int) []Tick {
	if target < 2 {
		target = 5
	}
	step := niceStep(s.Lo, s.Hi, target)
	var ticks []Tick
	start := math.Ceil(s.Lo/step) * step
	for v := start; v <= s.Hi+step*1e-9; v += step {
		if math.Abs(v) < step*1e-9 {
			v = 0 // clean up -0
		}
		ticks = append(ticks, Tick{Value: v, Pos: s.Map(v)})
	}
	return ticks
}

// Band is a categorical scale: N evenly spaced bands, each with a center and a
// width. Inner is the fraction of each band left empty as padding (0..1).
type Band struct {
	Cats  []string
	Inner float32
}

// NewBand builds a band scale over the given category labels.
func NewBand(cats []string) *Band { return &Band{Cats: cats, Inner: 0.3} }

func (s *Band) n() int { return len(s.Cats) }

func (s *Band) Map(v float64) float32 {
	n := s.n()
	if n == 0 {
		return 0
	}
	return (float32(v) + 0.5) / float32(n)
}

func (s *Band) Invert(t float32) float64 {
	n := s.n()
	if n == 0 {
		return 0
	}
	i := int(float64(t) * float64(n))
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	return float64(i)
}

func (s *Band) Domain() (float64, float64) { return -0.5, float64(s.n()) - 0.5 }

// Bandwidth is the fraction of the axis (0..1) occupied by one band's content
// (after Inner padding).
func (s *Band) Bandwidth() float32 {
	n := s.n()
	if n == 0 {
		return 0
	}
	return (1 - s.Inner) / float32(n)
}

func (s *Band) Ticks(_ int) []Tick {
	ticks := make([]Tick, s.n())
	for i, c := range s.Cats {
		ticks[i] = Tick{Value: float64(i), Pos: s.Map(float64(i)), Label: c}
	}
	return ticks
}

// bander is implemented by scales that have a discrete band width (Band), so a
// BarMark can size its bars to the category slot.
type bander interface{ Bandwidth() float32 }

// niceStep returns a 1/2/5×10ⁿ tick step that yields ~target intervals over
// [lo, hi].
func niceStep(lo, hi float64, target int) float64 {
	if target < 1 {
		target = 1
	}
	span := hi - lo
	if span <= 0 {
		return 1
	}
	raw := span / float64(target)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch norm := raw / mag; {
	case norm < 1.5:
		return mag
	case norm < 3:
		return 2 * mag
	case norm < 7:
		return 5 * mag
	default:
		return 10 * mag
	}
}

// fmtNumber renders a tick value compactly: no trailing zeros, thousands
// separators, and k/M suffixes for large magnitudes.
func fmtNumber(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 1e6:
		return trim(v/1e6) + "M"
	case a >= 1e3:
		return trim(v/1e3) + "k"
	default:
		return trim(v)
	}
}

func trim(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
