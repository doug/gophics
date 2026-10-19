package paint

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
)

// Measurement is on Canvas so a custom painter can size the text it is about
// to draw. The contract that makes it worth having is that the answer comes
// from the same Painter that will shape the run — a canvas whose measurements
// disagreed with its own TextIn would be worse than none, because the caller
// would lay out against a lie.

func measuringCanvas(t *testing.T) (*Painter, Canvas) {
	t.Helper()
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	return p, p.BeginOffscreen(geom.Size{W: 200, H: 60}, 1)
}

func TestCanvasMeasuresTheSameAsThePainterThatDrawsIt(t *testing.T) {
	p, c := measuringCanvas(t)
	const s = "Measure me"
	const size = 16

	if got, want := c.MeasureWidthIn("", s, size), p.MeasureWidthIn("", s, size); got != want {
		t.Errorf("canvas width %v, painter width %v — a canvas must agree with its own TextIn", got, want)
	}
	if got, want := c.MetricsIn("", size), p.MetricsIn("", size); got != want {
		t.Errorf("canvas metrics %+v, painter metrics %+v", got, want)
	}
	gotR, gotOK := c.InkBoundsIn("", s, size)
	wantR, wantOK := p.InkBoundsIn("", s, size)
	if gotR != wantR || gotOK != wantOK {
		t.Errorf("canvas ink %v,%v painter ink %v,%v", gotR, gotOK, wantR, wantOK)
	}
}

// A zero-returning stub satisfies the interface just as well as a real
// implementation, so assert the numbers are real: this is the test that fails
// if a Canvas is ever wired to nullMeasurer by accident.
func TestCanvasMeasurementIsNotStubbedToZero(t *testing.T) {
	_, c := measuringCanvas(t)
	const size = 16

	if w := c.MeasureWidthIn("", "wide text", size); w <= 0 {
		t.Errorf("width %v; want > 0", w)
	}
	if m := c.MetricsIn("", size); m.Ascent <= 0 || m.LineHeight() <= 0 {
		t.Errorf("metrics %+v; want positive ascent and line height", m)
	}
	if r, ok := c.InkBoundsIn("", "Hg", size); !ok || r.IsEmpty() {
		t.Errorf("ink bounds %v ok=%v; want a non-empty box", r, ok)
	}
}

// Width has to grow with the string and with the size, or a caller cannot use
// it to lay anything out.
func TestCanvasWidthGrowsWithTextAndSize(t *testing.T) {
	_, c := measuringCanvas(t)
	short := c.MeasureWidthIn("", "ii", 16)
	long := c.MeasureWidthIn("", "iiiiiiii", 16)
	if long <= short {
		t.Errorf("width did not grow with the string: %v then %v", short, long)
	}
	small := c.MeasureWidthIn("", "sample", 10)
	big := c.MeasureWidthIn("", "sample", 30)
	if big <= small {
		t.Errorf("width did not grow with the size: %v then %v", small, big)
	}
}

// The advance box and the ink box are different questions, and a caller
// choosing between them needs them to actually differ: "Hg" has a descender
// below the baseline and the ink box has to reach it.
func TestCanvasInkBoundsStraddleTheBaseline(t *testing.T) {
	_, c := measuringCanvas(t)
	r, ok := c.InkBoundsIn("", "Hg", 24)
	if !ok {
		t.Fatal("no ink for \"Hg\"")
	}
	if r.Min.Y >= 0 {
		t.Errorf("ink top %v; want above the baseline (negative)", r.Min.Y)
	}
	if r.Max.Y <= 0 {
		t.Errorf("ink bottom %v; want below the baseline for a descender", r.Max.Y)
	}
}

// Whitespace advances the pen but paints nothing: ok=false is how a caller
// learns there is no ink to damage or to fit a box around.
func TestCanvasInkBoundsReportsNothingForBlankText(t *testing.T) {
	_, c := measuringCanvas(t)
	if _, ok := c.InkBoundsIn("", "   ", 16); ok {
		t.Error("spaces reported ink")
	}
	if w := c.MeasureWidthIn("", "   ", 16); w <= 0 {
		t.Errorf("spaces advanced %v; want > 0 — they still take room", w)
	}
}
