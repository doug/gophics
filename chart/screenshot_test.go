package chart

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// shot renders root to a PNG at path for visual inspection.
func shot(t *testing.T, path string, size geom.Size, bg paint.Color, root widget.Widget) {
	t.Helper()
	h, err := app.NewHeadless(root, app.Config{
		Size: size, Background: bg,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.Render()); err != nil {
		t.Fatal(err)
	}
}

// TestChartShot renders sample charts. Run:
//
//	CHART_SHOT=<dir> go test -run TestChartShot ./chart
func TestChartShot(t *testing.T) {
	dir := os.Getenv("CHART_SHOT")
	if dir == "" {
		t.Skip("set CHART_SHOT=<dir>")
	}
	white := paint.RGB(1, 1, 1)

	bar := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{BarMark{Data: Values(
			"Mon", 12, "Tue", 19, "Wed", 7, "Thu", 22, "Fri", 15, "Sat", 9, "Sun", 4)}},
	}}
	shot(t, dir+"/bar.png", geom.Size{W: 720, H: 420}, white, bar)

	line := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{
			RuleMark{Value: 50, Horizontal: true, Dash: 5},
			LineMark{Data: XY(0, 20, 1, 35, 2, 30, 3, 55, 4, 48, 5, 70, 6, 62, 7, 85), Points: true},
		},
		XAxis: Axis{Grid: true},
	}}
	shot(t, dir+"/line.png", geom.Size{W: 720, H: 420}, white, line)
}
