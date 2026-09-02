package main

import (
	"github.com/doug/gophics/chart"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// BG is the window background used at Start, before a widget context exists (the
// app passes it as Config.Background). Inside the tree every color comes from
// theme.Of(ctx), so the dashboard also follows the platform light/dark scheme.
var BG = theme.Light().Bg

// Ledger is the dashboard root: a stateless widget that lays out a few chart
// cards over the sample dataset.
type Ledger struct{}

func (Ledger) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme and provide it to the tree
	// so every card and chart below reads colors from it and follows light/dark.
	th := theme.Auto(ctx)
	d := sampleData()

	// Categorical series colors come from the theme's chart palette; the budget
	// threshold uses the semantic Warning token. Chart chrome (labels, axes,
	// gridlines) is themed via the chart's Chrome overrides so it tracks light/dark.
	balAccent := th.ChartAt(0)
	balance := chart.Chart{
		Marks: []chart.Mark{
			chart.AreaMark{Data: d.balance, Color: balAccent, Alpha: 0.13},
			chart.RuleMark{Value: d.budget, Horizontal: true, Dash: 6, Color: th.Warning},
			chart.LineMark{Data: d.balance, Width: 2.5, Color: balAccent},
		},
		X:          chart.NewTime(d.start, d.end),
		LabelColor: th.Text, AxisColor: th.Muted, GridColor: th.Border,
		Animate: true,
	}
	// One categorical accent per spending slice, wrapping the 6-color palette.
	sliceColors := make([]paint.Color, len(d.byCategory))
	for i := range d.byCategory {
		sliceColors[i] = th.ChartAt(i)
	}
	donut := chart.Chart{
		Marks:      []chart.Mark{chart.SectorMark{Inner: 0.58, Data: d.byCategory, Colors: sliceColors}},
		XAxis:      chart.Axis{Hide: true},
		YAxis:      chart.Axis{Hide: true},
		LabelColor: th.Text,
		Legend:     true,
		Animate:    true,
	}
	week := chart.Chart{
		Marks:      []chart.Mark{chart.BarMark{Data: d.thisWeek, Color: th.ChartAt(1)}},
		LabelColor: th.Text, AxisColor: th.Muted, GridColor: th.Border,
		Animate: true,
	}

	row := widget.Row(
		widget.Flexible{Flex: 3, Child: card(th, "Where it goes", "", 300, donut)},
		widget.Sized{W: 16},
		widget.Flexible{Flex: 2, Child: card(th, "This week", "", 300, week)},
	)
	row.CrossAlign = layout.CrossStretch

	page := column(layout.CrossStretch,
		header(th, "Ledger", "August 2026"),
		widget.Sized{H: 18},
		card(th, "Balance", money(d.latest), 260, balance),
		widget.Sized{H: 16},
		row,
	)
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: widget.Padding{All: 24, Child: page}},
	}
}

// header is the page title + subtitle.
func header(th theme.Theme, title, sub string) widget.Widget {
	return column(layout.CrossStart,
		widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Display, Color: th.Text},
		widget.Sized{H: 3},
		widget.Text{Value: sub, Size: th.Type.Body, Color: th.Muted},
	)
}

// card is a titled surface panel of fixed height wrapping a chart (or any body).
// A non-empty value is shown large under the title.
func card(th theme.Theme, title, value string, h float32, body widget.Widget) widget.Widget {
	head := []widget.Widget{widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text}}
	if value != "" {
		head = append(head,
			widget.Sized{H: 4},
			widget.Text{Value: value, Font: theme.FontBold, Size: th.Type.Display, Color: th.Text})
	}
	head = append(head, widget.Sized{H: 12}, widget.Expand(body))
	return widget.Sized{H: h, Child: widget.Decorated{
		Color: th.Surface, Radius: th.Radius + 6, BorderColor: th.Border, BorderWidth: 1,
		Child: widget.Padding{All: 20, Child: column(layout.CrossStretch, head...)},
	}}
}

func column(cross layout.CrossAlign, kids ...widget.Widget) widget.Widget {
	c := widget.Column(kids...)
	c.CrossAlign = cross
	return c
}
