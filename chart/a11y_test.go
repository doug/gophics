package chart

import (
	"github.com/doug/gophics/intl"

	"strings"
	"testing"
)

func TestSemanticsLabel(t *testing.T) {
	cases := []struct {
		name    string
		chart   Chart
		wantSub []string
	}{
		{"bar", Chart{Marks: []Mark{BarMark{Data: Values("a", 1, "b", 2)}}},
			[]string{"Bar chart", "a 1", "b 2"}},
		{"line", Chart{Marks: []Mark{LineMark{Data: XY(0, 1, 1, 2, 2, 3)}}},
			[]string{"Line chart", "3 points"}},
		{"pie", Chart{Marks: []Mark{SectorMark{Data: Values("x", 5, "y", 3)}}},
			[]string{"Pie chart", "x 5"}},
		{"heatmap", Chart{Marks: []Mark{RectMark{Cells: []Cell{{}, {}, {}}}}},
			[]string{"Heatmap", "3 cells"}},
		// A range chart is not selectable and used to fall through to "",
		// invisible to a screen reader while every other kind was described.
		{"range", Chart{Marks: []Mark{RangeMark{Data: []Span{{X: 0, Lo: 2, Hi: 5}, {X: 1, Lo: 1, Hi: 9}}}}},
			[]string{"Range chart", "2 spans", "1 to 9"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.chart.semanticsLabel(intl.Default)
			for _, sub := range c.wantSub {
				if !strings.Contains(got, sub) {
					t.Fatalf("label %q missing %q", got, sub)
				}
			}
		})
	}
}
