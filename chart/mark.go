package chart

import (
	"fmt"
	"log"

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

// Pair is one categorical label/value pair, the typed input to Pairs.
type Pair struct {
	Label string
	Value float64
}

// Pairs builds categorical data from typed label/value pairs — the
// compile-time-checked counterpart to Values:
//
//	chart.Pairs([]chart.Pair{{"Mon", 3}, {"Tue", 5}, {"Wed", 2}})
//
// Each label becomes a band; X is the band index.
func Pairs(pairs []Pair) []Datum {
	out := make([]Datum, len(pairs))
	for i, p := range pairs {
		out[i] = Datum{X: float64(i), Y: p.Value, Label: p.Label}
	}
	return out
}

// Values builds categorical data from alternating label, number pairs:
//
//	chart.Values("Mon", 3, "Tue", 5, "Wed", 2)
//
// Each label becomes a band; X is the band index; the value may be any int/uint/
// float type. It is prototyping sugar — the variadic ...any means the compiler
// cannot check the arguments; use Pairs for the typed, compile-time-checked
// path. A trailing odd argument is ignored with a logged warning; a non-string
// label or non-numeric value still panics (a programming error in a
// literal-args helper, like a bad fmt verb, surfaced immediately rather than
// silently dropped).
func Values(pairs ...any) []Datum {
	if len(pairs)%2 != 0 {
		log.Printf("chart.Values: odd argument count (want label, value pairs); ignoring trailing %T", pairs[len(pairs)-1])
		pairs = pairs[:len(pairs)-1]
	}
	out := make([]Datum, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		label, ok := pairs[i].(string)
		if !ok {
			panic(fmt.Sprintf("chart.Values: argument %d is not a string label: %T", i, pairs[i]))
		}
		y, ok := toFloat(pairs[i+1])
		if !ok {
			panic(fmt.Sprintf("chart.Values: value for %q is not numeric: %T", label, pairs[i+1]))
		}
		out = append(out, Datum{X: float64(len(out)), Y: y, Label: label})
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
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
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

// plot is the resolved drawing context handed to each mark: the pixel rect of
// the plot area, the axis scales, the canvas, the theme, and the 0..1 animation
// progress.
type plot struct {
	Area   geom.Rect
	X, Y   Scale
	Canvas paint.Canvas
	th     chartTheme
	series int     // this mark's series index (for the default color)
	group  int     // this bar series' index among grouped bars
	groups int     // total grouped bar series (1 = ungrouped)
	T      float32 // animation progress; 1 when settled
}

// px maps a domain X to a pixel x within the plot area.
func (p plot) px(v float64) float32 { return p.Area.Min.X + p.X.Map(v)*p.Area.Dx() }

// py maps a domain Y to a pixel y (inverted: larger Y is higher on screen).
func (p plot) py(v float64) float32 { return p.Area.Max.Y - p.Y.Map(v)*p.Area.Dy() }

// Mark is one visual layer of a chart. Domain methods feed scale inference;
// draw renders the mark against resolved scales.
type Mark interface {
	xDomain() (lo, hi float64, cats []string)
	yDomain() (lo, hi float64)
	draw(p plot)
}

// named marks contribute an entry to the legend (empty name → omitted).
type named interface{ markName() string }

// seriesSlot is the pixel width one item may occupy: a band's bandwidth, or the
// smallest pixel gap between adjacent x positions.
func seriesSlot(p plot, xs []float64) float32 {
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
