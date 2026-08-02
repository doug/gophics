package main

import (
	"github.com/doug/gossamer/chart"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

var (
	colBG   = paint.RGB(0.95, 0.96, 0.97)
	colCard = paint.RGB(1, 1, 1)
	colInk  = paint.RGB(0.11, 0.13, 0.18)
	colSub  = paint.RGB(0.45, 0.49, 0.55)
	colWarn = paint.RGB(0.86, 0.26, 0.28)
	colGood = paint.RGB(0.16, 0.63, 0.40)
)

// Ledger is the dashboard root: a stateless widget that lays out a few chart
// cards over the sample dataset.
type Ledger struct{}

func (Ledger) Build(_ widget.Ctx) widget.Widget {
	d := sampleData()

	orange := paint.RGB(0.95, 0.56, 0.16)
	balance := chart.Chart{
		Marks: []chart.Mark{
			chart.AreaMark{Data: d.balance, Color: orange, Alpha: 0.13},
			chart.RuleMark{Value: d.budget, Horizontal: true, Dash: 6, Color: colWarn},
			chart.LineMark{Data: d.balance, Width: 2.5, Color: orange},
		},
		XAxis:   chart.Axis{Format: d.dayLabel},
		Animate: true,
	}
	spend := chart.Chart{
		Marks:   []chart.Mark{chart.BarMark{Data: d.byCategory, Color: paint.RGB(0.20, 0.47, 0.85)}},
		Animate: true,
	}
	week := chart.Chart{
		Marks:   []chart.Mark{chart.BarMark{Data: d.thisWeek, Color: colGood}},
		Animate: true,
	}

	page := column(layout.CrossStretch,
		header("Ledger", "August 2026"),
		widget.Sized{H: 18},
		card("Balance", money(d.latest), 280, balance),
		widget.Sized{H: 16},
		rowCards(
			card("Spending by category", "", 260, spend),
			card("This week", "", 260, week),
		),
	)
	return widget.Decorated{Color: colBG, Child: widget.Padding{All: 24, Child: page}}
}

// header is the page title + subtitle.
func header(title, sub string) widget.Widget {
	return column(layout.CrossStart,
		widget.Text{S: title, Font: "bold", Size: 30, Color: colInk},
		widget.Sized{H: 3},
		widget.Text{S: sub, Size: 15, Color: colSub},
	)
}

// card is a titled white panel of fixed height wrapping a chart (or any body).
// A non-empty value is shown large under the title.
func card(title, value string, h float32, body widget.Widget) widget.Widget {
	head := []widget.Widget{widget.Text{S: title, Font: "bold", Size: 16, Color: colInk}}
	if value != "" {
		head = append(head,
			widget.Sized{H: 4},
			widget.Text{S: value, Font: "bold", Size: 30, Color: colInk})
	}
	head = append(head, widget.Sized{H: 12}, widget.Expand(body))
	return widget.Sized{H: h, Child: widget.Decorated{
		Color: colCard, Radius: 16,
		Child: widget.Padding{All: 20, Child: column(layout.CrossStretch, head...)},
	}}
}

// rowCards places two cards side by side, sharing the width evenly.
func rowCards(a, b widget.Widget) widget.Widget {
	r := widget.Row(widget.Expand(a), widget.Sized{W: 16}, widget.Expand(b))
	r.CrossAlign = layout.CrossStretch
	return r
}

func column(cross layout.CrossAlign, kids ...widget.Widget) widget.Widget {
	c := widget.Column(kids...)
	c.CrossAlign = cross
	return c
}
