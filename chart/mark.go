package chart

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Datum is one data point. X is numeric (or a band index for categorical marks);
// Label is the categorical name (optional); Color overrides the series color when
// its alpha is non-zero.
type Datum struct {
	X, Y  float64
	Label string
	Color paint.Color
}

// Values builds categorical data from alternating label, number pairs:
//
//	chart.Values("Mon", 3, "Tue", 5, "Wed", 2)
//
// Each label becomes a band; X is the band index. Non-string labels or
// non-number values are skipped.
func Values(pairs ...any) []Datum {
	var out []Datum
	for i := 0; i+1 < len(pairs); i += 2 {
		label, ok := pairs[i].(string)
		if !ok {
			continue
		}
		if y, ok := toFloat(pairs[i+1]); ok {
			out = append(out, Datum{X: float64(len(out)), Y: y, Label: label})
		}
	}
	return out
}

// XY builds continuous data from alternating x, y numbers.
func XY(nums ...float64) []Datum {
	var out []Datum
	for i := 0; i+1 < len(nums); i += 2 {
		out = append(out, Datum{X: nums[i], Y: nums[i+1]})
	}
	return out
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}

// cats returns the category labels of d if it is categorical (every datum has a
// Label), else nil.
func cats(d []Datum) []string {
	if len(d) == 0 {
		return nil
	}
	out := make([]string, len(d))
	for i, x := range d {
		if x.Label == "" {
			return nil
		}
		out[i] = x.Label
	}
	return out
}

// Plot is the resolved drawing context handed to each mark: the pixel rect of
// the plot area, the axis scales, the canvas, the theme, and the 0..1 animation
// progress.
type Plot struct {
	Area   geom.Rect
	X, Y   Scale
	Canvas paint.Canvas
	th     theme
	series int     // this mark's series index (for the default color)
	group  int     // this bar series' index among grouped bars
	groups int     // total grouped bar series (1 = ungrouped)
	T      float32 // animation progress; 1 when settled
}

// px maps a domain X to a pixel x within the plot area.
func (p Plot) px(v float64) float32 { return p.Area.Min.X + p.X.Map(v)*p.Area.Dx() }

// py maps a domain Y to a pixel y (inverted: larger Y is higher on screen).
func (p Plot) py(v float64) float32 { return p.Area.Max.Y - p.Y.Map(v)*p.Area.Dy() }

// Mark is one visual layer of a chart. Domain methods feed scale inference;
// draw renders the mark against resolved scales.
type Mark interface {
	xDomain() (lo, hi float64, cats []string)
	yDomain() (lo, hi float64)
	draw(p Plot)
}

// named marks contribute an entry to the legend (empty name → omitted).
type named interface{ markName() string }

// seriesSlot is the pixel width one item may occupy: a band's bandwidth, or the
// smallest pixel gap between adjacent x positions.
func seriesSlot(p Plot, xs []float64) float32 {
	if bd, ok := p.X.(bander); ok {
		return bd.Bandwidth() * p.Area.Dx()
	}
	if len(xs) < 2 {
		return p.Area.Dx() * 0.4
	}
	minGap := p.Area.Dx()
	for i := 1; i < len(xs); i++ {
		if g := abs(p.px(xs[i]) - p.px(xs[i-1])); g > 0 && g < minGap {
			minGap = g
		}
	}
	return minGap
}
