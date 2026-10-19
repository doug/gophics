package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// The canvas a widget is handed during a frame is the scene recorder, not the
// painter's canvas — so measurement has to work on that one too, or the
// feature exists only in unit tests. This is the end-to-end claim: inside
// Canvas.Draw, a custom painter can ask how wide its label is and get a real
// answer.
func TestCustomPainterCanMeasureTheTextItDraws(t *testing.T) {
	var width float32
	var metrics paint.TextMetrics
	var ink geom.Rect
	var inkOK bool

	a := apptest.New(t, widget.Canvas{
		W: 200, H: 80,
		Draw: func(c paint.Canvas, size geom.Size) {
			const label = "centered"
			width = c.MeasureWidthIn("", label, 16)
			metrics = c.MetricsIn("", 16)
			ink, inkOK = c.InkBoundsIn("", label, 16)
			// The point of measuring: place the text using the answer.
			c.TextIn("", label, geom.Pt{
				X: (size.W - width) / 2,
				Y: (size.H + metrics.Ascent) / 2,
			}, 16, paint.RGB(0, 0, 0))
		},
	}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 200, H: 80},
		Font: goregular.TTF, // measurement has no answer without a font
	}))
	a.Render()

	if width <= 0 {
		t.Errorf("measured width %v inside Draw; want > 0", width)
	}
	if metrics.Ascent <= 0 {
		t.Errorf("measured ascent %v inside Draw; want > 0", metrics.Ascent)
	}
	if !inkOK || ink.IsEmpty() {
		t.Errorf("measured ink %v ok=%v inside Draw; want a non-empty box", ink, inkOK)
	}
}
