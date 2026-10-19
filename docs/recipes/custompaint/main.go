// Command custompaint is the cookbook's custom-painter recipe: a small bar
// chart drawn entirely inside one widget.Canvas, with its labels positioned
// from measured text rather than from guessed widths.
//
//	go run ./docs/recipes/custompaint
//
// Measuring is the part that was until recently impossible. paint.Canvas now
// embeds paint.Measurer, so the canvas handed to Draw can be asked how wide a
// run will be before it is drawn — and it answers from the same painter that
// will shape it, so the measurement and the draw cannot disagree. Before that,
// a custom painter had to assume an average glyph width, which is wrong for
// every string and visibly wrong for digits beside letters.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type bar struct {
	label string
	value float32
}

var bars = []bar{
	{"Desktop", 0.82},
	{"Web", 0.64},
	{"iOS", 0.47},
	{"Android", 0.55},
	{"Terminal", 0.21},
}

// Chart is stateless: it has no mutable state, so it implements Build
// directly rather than CreateState. Most custom painters are like this —
// the drawing is a pure function of the data above it.
type Chart struct{}

func (Chart) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	return widget.Canvas{H: 220, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
		const (
			size0 = float32(12) // label size
			gap   = float32(10) // between label and bar
			rowH  = float32(34)
		)

		// Measure every label and lay the chart out around the widest, so the
		// bars all start at the same x and no label is clipped. This is the
		// whole reason measurement belongs on the canvas: the gutter is a
		// property of the text, not a number someone picked.
		gutter := float32(0)
		for _, b := range bars {
			if w := c.MeasureWidthIn("", b.label, size0); w > gutter {
				gutter = w
			}
		}

		// MetricsIn gives the vertical metrics of the face at this size.
		// Centring text on a row means centring the ink between ascent and
		// descent — not placing the baseline at the row's middle, which sits
		// every label noticeably low.
		m := c.MetricsIn("", size0)
		baseline := (rowH + m.Ascent - m.Descent) / 2

		trackX := gutter + gap
		trackW := size.W - trackX
		if trackW < 1 {
			return // too narrow to draw anything honest
		}

		for i, b := range bars {
			top := float32(i) * rowH

			// Right-align the label against the gutter.
			lw := c.MeasureWidthIn("", b.label, size0)
			c.TextIn("", b.label, geom.Pt{X: gutter - lw, Y: top + baseline}, size0, th.Muted)

			track := geom.RectXYWH(trackX, top+9, trackW, 16)
			c.FillRRect(track, 8, th.SurfaceHover)
			c.FillRRect(geom.RectXYWH(trackX, top+9, trackW*b.value, 16), 8, th.ChartAt(i))

			// The value sits just inside the bar's end when it fits there, and
			// just outside when it does not — a decision that needs the width
			// of the text and the width of the bar in the same breath.
			v := fmt.Sprintf("%.0f%%", b.value*100)
			vw := c.MeasureWidthIn("", v, size0)
			x, col := trackX+trackW*b.value-vw-8, th.OnPrimary
			if x < trackX+4 {
				x, col = trackX+trackW*b.value+8, th.Muted
			}
			c.TextIn("", v, geom.Pt{X: x, Y: top + baseline}, size0, col)
		}
	}}
}

type Root struct{}

func (Root) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)
	col := widget.Column(
		theme.Title("Platform coverage"),
		widget.Sized{H: 6},
		theme.Caption("drawn in one Canvas, laid out from measured text"),
		widget.Sized{H: 16},
		theme.Card{Child: Chart{}},
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 20, Child: col}}}
}

func main() {
	if err := app.Run(Root{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · custom painter",
		Size:           geom.Size{W: 440, H: 340},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
