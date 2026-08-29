package chart

import (
	"fmt"
	"strings"

	"github.com/doug/gophics/intl"
)

// semanticsLabel produces a spoken summary of the chart's data for the
// accessibility tree (a Canvas is otherwise invisible to screen readers).
func (w Chart) semanticsLabel(loc intl.Locale) string {
	for _, mk := range w.Marks {
		if sm, ok := mk.(SectorMark); ok && len(sm.Data) > 0 {
			return "Pie chart. " + summarize(sm.Data, loc)
		}
	}
	for _, mk := range w.Marks {
		sd, ok := mk.(selectable)
		if !ok || len(sd.seriesData()) == 0 {
			continue
		}
		kind := "Chart"
		switch mk.(type) {
		case BarMark:
			kind = "Bar chart"
		case LineMark:
			kind = "Line chart"
		case AreaMark:
			kind = "Area chart"
		case PointMark:
			kind = "Scatter chart"
		}
		return kind + ". " + summarize(sd.seriesData(), loc)
	}
	for _, mk := range w.Marks {
		if rm, ok := mk.(RectMark); ok {
			return fmt.Sprintf("Heatmap, %d cells.", len(rm.Cells))
		}
		if cm, ok := mk.(CandleMark); ok {
			return fmt.Sprintf("Candlestick chart, %d bars.", len(cm.Data))
		}
	}
	return ""
}

// summarize lists categorical points (capped), or the count and value range for
// continuous data.
func summarize(d []Datum, loc intl.Locale) string {
	if len(d) == 0 {
		return ""
	}
	if cats(d) != nil {
		var b strings.Builder
		n := min(len(d), 10)
		for i := range n {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s %s", d[i].Label, fmtNumber(d[i].Y, loc))
		}
		if len(d) > 10 {
			b.WriteString(", and more")
		}
		return b.String()
	}
	lo, hi := minMaxY(d)
	return fmt.Sprintf("%d points, %s to %s", len(d), fmtNumber(lo, loc), fmtNumber(hi, loc))
}
