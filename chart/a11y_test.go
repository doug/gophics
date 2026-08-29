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
