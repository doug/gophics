package ui

import (
	"github.com/doug/gophics/chart"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// chartsSection shows several chart marks over realistic small datasets, each
// themed to the active palette so a re-theme restyles the charts too.
type chartsSection struct{}

func (chartsSection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	// Shared chrome: axis/label/grid colors and the series palette come from the
	// theme, so every chart matches the surrounding app in light and dark.
	chrome := func(c chart.Chart) chart.Chart {
		c.LabelColor = th.Text
		c.AxisColor = th.Muted
		c.GridColor = th.Border
		c.Palette = th.Chart[:]
		return c
	}

	revenue := chart.XY(0, 12, 1, 18, 2, 15, 3, 22, 4, 28, 5, 26, 6, 34)

	lineArea := chrome(chart.Chart{
		Legend: true,
		Marks: []chart.Mark{
			chart.AreaMark{Data: revenue, Name: "Revenue", Alpha: 0.18},
			chart.LineMark{Data: revenue, Name: "Revenue", Width: 2.5, Smooth: true, Points: true},
		},
	})

	// chart.Pairs is the typed constructor (chart.Values is the same as
	// variadic prototyping sugar).
	bars := chrome(chart.Chart{
		Marks: []chart.Mark{
			chart.BarMark{Data: chart.Pairs([]chart.Pair{
				{Label: "Mon", Value: 5}, {Label: "Tue", Value: 8}, {Label: "Wed", Value: 6},
				{Label: "Thu", Value: 9}, {Label: "Fri", Value: 12}, {Label: "Sat", Value: 4},
				{Label: "Sun", Value: 3},
			})},
		},
	})

	pie := chrome(chart.Chart{
		Legend: true,
		XAxis:  chart.Axis{Hide: true},
		YAxis:  chart.Axis{Hide: true},
		Marks: []chart.Mark{
			chart.SectorMark{Inner: 0.55, Data: chart.Values("Direct", 42, "Search", 28, "Social", 18, "Email", 12)},
		},
	})

	heatmap := chrome(chart.Chart{
		XAxis: chart.Axis{Hide: true},
		YAxis: chart.Axis{Hide: true},
		Marks: []chart.Mark{
			chart.RectMark{Cells: activityCells(), Cols: 10, Rows: 5,
				Scale: chart.ColorScale{Lo: 0, Hi: 9,
					From: th.Surface, To: th.Primary}},
		},
	})

	return sectionColumn(
		chartCard(th, "Line + area", "Smoothed revenue with a filled area and a legend", 190, lineArea),
		widget.Sized{H: 14},
		chartCard(th, "Bar", "Weekly counts over a categorical band scale", 190, bars),
		widget.Sized{H: 14},
		chartCard(th, "Donut", "Traffic sources as a Sector mark with a legend", 210, pie),
		widget.Sized{H: 14},
		chartCard(th, "Heatmap", "A contribution grid of Rect cells over a color scale", 170, heatmap),
	)
}

// chartCard frames one chart with a heading and a fixed-height plot area.
func chartCard(th theme.Theme, title, subtitle string, height float32, c chart.Chart) widget.Widget {
	head := widget.Column(
		widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text},
		widget.Sized{H: 2},
		widget.Text{Value: subtitle, Size: th.Type.Caption, Color: th.Muted},
	)
	head.CrossAlign = layout.CrossStart
	body := widget.Column(
		head,
		widget.Sized{H: 10},
		widget.Sized{H: height, Child: c},
	)
	body.CrossAlign = layout.CrossStretch
	// Solid: a frosted card over a chart costs the whole chart drawn again,
	// every frame, and a chart reads better over a steady background anyway.
	return theme.Card{Solid: true, Child: body}
}

// activityCells builds a deterministic 10×5 grid of activity values (0..9) for
// the heatmap — a plausible little contribution graph.
func activityCells() []chart.Cell {
	cells := make([]chart.Cell, 0, 50)
	for x := range 10 {
		for y := range 5 {
			v := (x*7 + y*3) % 10 // deterministic spread across the ramp
			cells = append(cells, chart.Cell{X: x, Y: y, V: float64(v)})
		}
	}
	return cells
}
