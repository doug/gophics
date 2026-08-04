package chart

import (
	"image/png"
	"math"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
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

	candles := make([]Candle, 14)
	prev := 50.0
	for i := range candles {
		o := prev
		cl := o + float64((i*37)%13) - 6
		hi := math.Max(o, cl) + float64((i*13)%5)
		lo := math.Min(o, cl) - float64((i*7)%5)
		candles[i] = Candle{X: float64(i), Open: o, High: hi, Low: lo, Close: cl}
		prev = cl
	}
	candle := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{CandleMark{Data: candles}},
	}}
	shot(t, dir+"/candle.png", geom.Size{W: 720, H: 420}, white, candle)

	rng := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{RangeMark{Data: []Span{
			{X: 0, Lo: 20, Hi: 65, Label: "Jan"}, {X: 1, Lo: 35, Hi: 80, Label: "Feb"},
			{X: 2, Lo: 28, Hi: 72, Label: "Mar"}, {X: 3, Lo: 40, Hi: 95, Label: "Apr"},
			{X: 4, Lo: 30, Hi: 60, Label: "May"},
		}}},
	}}
	shot(t, dir+"/range.png", geom.Size{W: 620, H: 380}, white, rng)

	var cells []Cell
	for x := 0; x < 20; x++ {
		for y := 0; y < 7; y++ {
			cells = append(cells, Cell{X: x, Y: y, V: float64((x*3 + y*5 + x*y) % 6)})
		}
	}
	heat := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{RectMark{Cells: cells, Cols: 20, Rows: 7, Scale: ColorScale{Lo: 0, Hi: 5}}},
		XAxis: Axis{Hide: true}, YAxis: Axis{Hide: true},
	}}
	shot(t, dir+"/heatmap.png", geom.Size{W: 720, H: 300}, white, heat)

	smooth := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{
			AreaMark{Data: XY(0, 20, 1, 35, 2, 22, 3, 55, 4, 40, 5, 72, 6, 58, 7, 85), Alpha: 0.14},
			LineMark{Data: XY(0, 20, 1, 35, 2, 22, 3, 55, 4, 40, 5, 72, 6, 58, 7, 85), Width: 3, Smooth: true, Points: true},
		},
	}}
	shot(t, dir+"/smooth.png", geom.Size{W: 720, H: 420}, white, smooth)

	donut := widget.Padding{Insets: geom.InsetsAll(24), Child: Chart{
		Marks: []Mark{SectorMark{Inner: 0.55, Data: Values(
			"Rent", 1850, "Food", 620, "Transit", 180, "Fun", 240, "Utilities", 165, "Shopping", 310)}},
		XAxis:  Axis{Hide: true},
		YAxis:  Axis{Hide: true},
		Legend: true,
	}}
	shot(t, dir+"/donut.png", geom.Size{W: 560, H: 460}, white, donut)

	grouped := widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{
			BarMark{Name: "Budget", Data: Values("Rent", 1800, "Food", 600, "Fun", 300, "Transit", 200)},
			BarMark{Name: "Actual", Data: Values("Rent", 1850, "Food", 720, "Fun", 240, "Transit", 180)},
		},
		Legend: true,
	}}
	shot(t, dir+"/grouped.png", geom.Size{W: 720, H: 440}, white, grouped)

	// Selection tooltip: mount, press a point, then render.
	size := geom.Size{W: 720, H: 420}
	h, err := app.NewHeadless(widget.Padding{Insets: geom.InsetsAll(28), Child: Chart{
		Marks: []Mark{BarMark{Data: Values(
			"Mon", 12, "Tue", 19, "Wed", 7, "Thu", 22, "Fri", 15, "Sat", 9, "Sun", 4)}},
	}}, app.Config{Size: size, Background: white,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 380, Y: 200}) // press the Thu bar
	h.Render()
	f, _ := os.Create(dir + "/select.png")
	defer f.Close()
	_ = png.Encode(f, h.Render())
}
