package chart

import "github.com/doug/gossamer/paint"

// theme is the resolved set of chrome colors + the series palette for a given
// light/dark mode.
type theme struct {
	text   paint.Color // tick labels, axis titles
	axis   paint.Color // axis baselines
	grid   paint.Color // gridlines
	series []paint.Color
}

func gray(v, a float32) paint.Color { return paint.Color{R: v, G: v, B: v, A: a} }

// lightSeries / darkSeries are pleasant categorical palettes (blue, orange,
// green, red, purple, teal, pink, yellow-green).
var lightSeries = []paint.Color{
	paint.RGB(0.20, 0.47, 0.85),
	paint.RGB(0.95, 0.56, 0.16),
	paint.RGB(0.22, 0.70, 0.44),
	paint.RGB(0.86, 0.26, 0.28),
	paint.RGB(0.56, 0.40, 0.80),
	paint.RGB(0.18, 0.68, 0.72),
	paint.RGB(0.90, 0.42, 0.62),
	paint.RGB(0.62, 0.68, 0.24),
}

var darkSeries = []paint.Color{
	paint.RGB(0.42, 0.64, 0.96),
	paint.RGB(0.98, 0.68, 0.35),
	paint.RGB(0.40, 0.82, 0.58),
	paint.RGB(0.95, 0.45, 0.47),
	paint.RGB(0.72, 0.58, 0.92),
	paint.RGB(0.38, 0.82, 0.86),
	paint.RGB(0.97, 0.58, 0.74),
	paint.RGB(0.78, 0.84, 0.42),
}

func themeFor(dark bool) theme {
	if dark {
		return theme{
			text:   gray(0.86, 1),
			axis:   gray(1, 0.34),
			grid:   gray(1, 0.10),
			series: darkSeries,
		}
	}
	return theme{
		text:   gray(0.16, 1),
		axis:   gray(0, 0.45),
		grid:   gray(0, 0.09),
		series: lightSeries,
	}
}

// color returns the series color at index i (wrapping), unless override has a
// non-zero alpha, in which case override wins.
func (t theme) color(i int, override paint.Color) paint.Color {
	if override.A > 0 {
		return override
	}
	return t.series[i%len(t.series)]
}
