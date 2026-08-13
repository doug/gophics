package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/doug/gophics/chart"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"

	"github.com/dougfritz/tally/book"
)

// overview is the dashboard: where you stand now, how you got here, and where the
// money went. It reads from the cached series computed in ensureSeries.
func (s *state) overviewView(th theme.Theme) widget.Widget {
	if len(s.netWorth) == 0 {
		return widget.Padding{All: 24, Child: widget.Text{
			S: "This ledger has no dated transactions to chart.", Color: th.Muted, Wrap: true,
		}}
	}

	latest := s.netWorth[len(s.netWorth)-1].Value
	col := widget.Column(
		s.summaryRow(th, latest),
		widget.Sized{H: 22},
		sectionLabel(th, "Net worth"),
		widget.Sized{H: 8},
		widget.Sized{H: 220, Child: s.netWorthChart(th)},
		s.unpricedNote(th),
		widget.Sized{H: 26},
		sectionLabel(th, "Income and expenses, by month"),
		widget.Sized{H: 8},
		widget.Sized{H: 190, Child: s.flowChart(th)},
		widget.Sized{H: 26},
		sectionLabel(th, "Where the money goes"),
		widget.Sized{H: 8},
		s.categoryTable(th),
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Expand(widget.Scroll{Child: widget.Padding{
		Insets: geom.Insets{Left: 24, Right: 24, Top: 12, Bottom: 28},
		Child:  col,
	}})
}

// summaryRow is the headline: current net worth plus the change over the series.
func (s *state) summaryRow(th theme.Theme, latest decimal.Decimal) widget.Widget {
	change := latest.Sub(s.netWorth[0].Value)
	sign := "+"
	if change.IsNegative() {
		sign = "−"
	}
	period := s.netWorth[0].Date.Format("Jan 2006") + " – " +
		s.netWorth[len(s.netWorth)-1].Date.Format("Jan 2006")

	return widget.Row(
		stat(th, "Net worth", fmtMoney(latest)+" "+s.baseCurrency, th.Text),
		widget.Sized{W: 40},
		stat(th, "Change", sign+fmtMoney(change.Abs())+" "+s.baseCurrency, changeColor(th, change)),
		widget.Expand(widget.Sized{W: 12}),
		stat(th, "Period", period, th.Muted),
	)
}

func stat(th theme.Theme, label, value string, col paint.Color) widget.Widget {
	c := widget.Column(
		widget.Text{S: label, Size: th.Type.Label, Color: th.Muted},
		widget.Sized{H: 3},
		widget.Text{S: value, Font: theme.FontBold, Size: th.Type.Title, Color: col},
	)
	c.CrossAlign = layout.CrossStart
	return c
}

// unpricedNote discloses holdings that carry no price, so the headline figure is
// never quietly understated. Silence would be the dishonest option.
func (s *state) unpricedNote(th theme.Theme) widget.Widget {
	if len(s.unpriced) == 0 {
		return widget.Sized{}
	}
	return widget.Padding{
		Insets: geom.Insets{Top: 6},
		Child: widget.Text{
			S: "Not included: " + strings.Join(s.unpriced, ", ") +
				" — held, but the ledger has no price for them in " + s.baseCurrency + ".",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true,
		},
	}
}

func sectionLabel(th theme.Theme, s string) widget.Widget {
	return widget.Text{S: s, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text}
}

// netWorthChart plots the cumulative balance as a filled area — one shape whose
// height *is* the quantity, which is what an area mark is for.
func (s *state) netWorthChart(th theme.Theme) widget.Widget {
	return chart.Chart{
		Marks: []chart.Mark{chart.AreaMark{
			Data:  points(s.netWorth),
			Name:  "Net worth",
			Color: th.Primary,
			Line:  2,
		}},
		X:          timeScale(s.netWorth),
		XAxis:      chart.Axis{Ticks: 6, Format: monthYear},
		YAxis:      chart.Axis{Ticks: 5, Format: compactMoney},
		LabelColor: th.Muted,
		AxisColor:  th.Border,
		GridColor:  th.Border,
	}
}

// flowChart overlays income and expenses so the gap between them — what you keep —
// is directly readable, rather than asking the eye to compare two separate panels.
func (s *state) flowChart(th theme.Theme) widget.Widget {
	return chart.Chart{
		Marks: []chart.Mark{
			chart.LineMark{Data: points(s.income), Name: "Income", Color: th.Success, Width: 2},
			chart.LineMark{Data: points(s.expenses), Name: "Expenses", Color: th.Danger, Width: 2},
		},
		X:          timeScale(s.income),
		XAxis:      chart.Axis{Ticks: 6, Format: monthYear},
		YAxis:      chart.Axis{Ticks: 4, Format: compactMoney},
		Legend:     true,
		LabelColor: th.Muted,
		AxisColor:  th.Border,
		GridColor:  th.Border,
	}
}

// categoryTable lists the biggest spending categories in the Tufte table, with a
// bar in each row sized to its share — the "small multiple" that turns a column of
// numbers into a comparison you can read at a glance.
func (s *state) categoryTable(th theme.Theme) widget.Widget {
	if len(s.categories) == 0 {
		return widget.Text{S: "No expenses recorded.", Size: th.Type.Body, Color: th.Muted}
	}
	max := s.categories[0].Total
	total := decimal.Zero
	for _, c := range s.categories {
		total = total.Add(c.Total)
	}

	return widget.Sized{
		H: float32(len(s.categories))*34 + 40,
		Child: theme.Table{
			Columns: []theme.Col{
				{Title: "Category", Flex: 2},
				{Title: "Share", Flex: 3},
				{Title: "Total", Width: 130, Align: theme.AlignEnd},
			},
			Count:     len(s.categories),
			RowHeight: 34,
			Cell: func(r, c int) widget.Widget {
				cat := s.categories[r]
				switch c {
				case 0:
					return widget.Text{S: shortCategory(cat.Name), Size: th.Type.Body, Color: th.Text,
						Ellipsis: true, MaxLines: 1}
				case 1:
					return shareBar(th, ratio(cat.Total, max))
				default:
					return widget.Text{S: fmtMoney(cat.Total), Font: "mono", Size: th.Type.Body, Color: th.Text}
				}
			},
		},
	}
}

// shareBar is a single proportional bar — a sparkline-class mark living inside a
// table cell, at the same resolution as the number beside it.
func shareBar(th theme.Theme, frac float64) widget.Widget {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := widget.Decorated{
		Color: th.Primary.WithAlpha(0.55), Radius: 3,
		Child: widget.Sized{H: 10},
	}
	rest := int(1000 * (1 - frac))
	bar := widget.Row(
		widget.Flexible{Flex: int(1000 * frac), Child: filled},
		widget.Flexible{Flex: max1(rest), Child: widget.Sized{H: 10}},
	)
	bar.CrossAlign = layout.CrossCenter
	return bar
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// points converts a book series into chart data, using Unix seconds for X so the
// time scale can format real dates.
func points(ps []book.Point) []chart.Datum {
	out := make([]chart.Datum, 0, len(ps))
	for _, p := range ps {
		v, _ := p.Value.Float64()
		out = append(out, chart.Datum{X: chart.Seconds(p.Date), Y: v})
	}
	return out
}

func timeScale(ps []book.Point) chart.Scale {
	if len(ps) == 0 {
		return nil
	}
	return chart.NewTime(ps[0].Date, ps[len(ps)-1].Date)
}

func monthYear(v float64) string {
	return time.Unix(int64(v), 0).UTC().Format("Jan '06")
}

// compactMoney keeps the y-axis readable: thousands as "12k", millions as "1.2M".
func compactMoney(v float64) string {
	abs := v
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000:
		return trimZero(v/1_000_000) + "M"
	case abs >= 1_000:
		return trimZero(v/1_000) + "k"
	default:
		return trimZero(v)
	}
}

// trimZero formats a number with at most one decimal place and no trailing ".0",
// so an axis reads "12k" rather than "12.0k".
func trimZero(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if strings.HasSuffix(s, ".0") {
		s = s[:len(s)-2]
	}
	return s
}

// ratio returns a/b as a float in [0,1], guarding a zero denominator.
func ratio(a, b decimal.Decimal) float64 {
	if b.IsZero() {
		return 0
	}
	f, _ := a.Div(b).Float64()
	return f
}

// shortCategory drops the leading "Expenses:" so the column reads as categories
// rather than repeating the same prefix on every row.
func shortCategory(name string) string {
	const prefix = "Expenses:"
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		return name[len(prefix):]
	}
	return name
}

func changeColor(th theme.Theme, d decimal.Decimal) paint.Color {
	if d.IsNegative() {
		return th.Danger
	}
	return th.Success
}

// ensureSeries computes the dashboard series once per loaded ledger; they walk
// every posting per month, so they must not be recomputed on every Build.
func (s *state) ensureSeries() {
	if s.seriesReady || s.book == nil {
		return
	}
	s.seriesReady = true
	s.baseCurrency = s.book.MainCurrency()
	s.netWorth = s.book.NetWorth(s.baseCurrency)
	s.income = s.book.MonthlyFlow("Income", s.baseCurrency)
	s.expenses = s.book.MonthlyFlow("Expenses", s.baseCurrency)
	s.categories = s.book.TopCategories(s.baseCurrency, 2, 8)
	s.unpriced = s.book.MissingPrices(s.baseCurrency)
}
