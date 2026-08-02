package chart

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// renderChart mounts a chart headless, settles its mount animation, and asserts
// it paints a non-empty image without panicking.
func renderChart(t *testing.T, root widget.Widget) {
	t.Helper()
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 420, H: 300}, Background: paint.RGB(1, 1, 1),
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		h.Step(0.05)
	}
	if h.Render().Bounds().Empty() {
		t.Fatal("empty render")
	}
}

func TestMarksRender(t *testing.T) {
	cases := map[string]Mark{
		"bar":   BarMark{Data: Values("a", 3, "b", 7, "c", 2, "d", 5)},
		"line":  LineMark{Data: XY(0, 1, 1, 3, 2, 2, 3, 4), Points: true},
		"point": PointMark{Data: XY(0, 1, 1, 3, 2, 2)},
		"rule":  RuleMark{Value: 2, Horizontal: true, Dash: 4},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			renderChart(t, Chart{Marks: []Mark{mk}, Animate: true, XAxis: Axis{Grid: true}})
		})
	}
}

// TestEmptyChart guards the degenerate case: no marks must not panic.
func TestEmptyChart(t *testing.T) {
	renderChart(t, Chart{})
}

// TestRangeAndCandle draws the interval and OHLC marks.
func TestRangeAndCandle(t *testing.T) {
	renderChart(t, Chart{Marks: []Mark{RangeMark{Data: []Span{
		{X: 0, Lo: 2, Hi: 8, Label: "a"}, {X: 1, Lo: 3, Hi: 6, Label: "b"},
	}}}, Animate: true})

	renderChart(t, Chart{Marks: []Mark{CandleMark{Data: []Candle{
		{X: 0, Open: 5, High: 8, Low: 4, Close: 7},
		{X: 1, Open: 7, High: 9, Low: 5, Close: 5},
	}}}, Animate: true})
}

// TestHeatmapRenders draws a small RectMark grid.
func TestHeatmapRenders(t *testing.T) {
	var cells []Cell
	for x := 0; x < 8; x++ {
		for y := 0; y < 5; y++ {
			cells = append(cells, Cell{X: x, Y: y, V: float64((x + y) % 4)})
		}
	}
	renderChart(t, Chart{
		Marks: []Mark{RectMark{Cells: cells, Scale: ColorScale{Lo: 0, Hi: 3}}},
		XAxis: Axis{Hide: true}, YAxis: Axis{Hide: true},
		Animate: true,
	})
}

// TestGroupedLegend renders grouped bars with a legend.
func TestGroupedLegend(t *testing.T) {
	renderChart(t, Chart{
		Marks: []Mark{
			BarMark{Name: "A", Data: Values("x", 3, "y", 5, "z", 2)},
			BarMark{Name: "B", Data: Values("x", 4, "y", 2, "z", 6)},
		},
		Legend:  true,
		Animate: true,
	})
}

// TestSelection drives presses on a bar chart and checks the nearest datum is
// selected (rightward press → higher index than a leftward one).
func TestSelection(t *testing.T) {
	var st *chartState
	stateHook = func(s *chartState) { st = s }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(
		Chart{Marks: []Mark{BarMark{Data: Values("a", 3, "b", 7, "c", 2, "d", 5)}}},
		app.Config{Size: geom.Size{W: 420, H: 300}, Background: paint.RGB(1, 1, 1),
			Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	stateHook = nil
	h.Render()
	if st == nil {
		t.Fatal("chart state not mounted")
	}

	h.Tap(geom.Pt{X: 40, Y: 150})
	h.Render()
	left := st.sel
	h.Tap(geom.Pt{X: 395, Y: 150})
	h.Render()
	right := st.sel
	if left < 0 || right < 0 {
		t.Fatalf("no selection: left=%d right=%d", left, right)
	}
	if right <= left {
		t.Fatalf("rightward press selected %d, should exceed leftward %d", right, left)
	}
}
