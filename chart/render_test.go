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
