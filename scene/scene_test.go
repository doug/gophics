package scene_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
)

// drawScene paints a representative scene: fills, gradient, stroke, line,
// text, and a nested clip.
func drawScene(c paint.Canvas) {
	c.Image(testImage, geom.RectXYWH(150, 10, 40, 30))
	c.Clear(paint.RGB(0.1, 0.1, 0.12))
	c.FillRect(geom.RectXYWH(10, 10, 100, 50), paint.RGB(0.9, 0.3, 0.3))
	c.FillRRect(geom.RectXYWH(30, 40, 120, 60), 12, paint.RGB(0.3, 0.9, 0.5))
	c.FillRRectGradient(geom.RectXYWH(60, 90, 100, 40), 8,
		paint.RGB(0.2, 0.4, 1), paint.RGB(1, 0.4, 0.2), false)
	c.StrokeRRect(geom.RectXYWH(20, 120, 80, 40), 6, 2, paint.RGB(1, 1, 1))
	c.Line(geom.Pt{X: 5, Y: 5}, geom.Pt{X: 195, Y: 155}, 1.5, paint.RGB(1, 1, 0))
	c.TextIn("", "scene", geom.Pt{X: 20, Y: 30}, 14, paint.RGB(1, 1, 1))
	c.PushClip(geom.RectXYWH(100, 100, 60, 60))
	c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.Color{R: 0.5, G: 0, B: 0.5, A: 0.5})
	c.PopClip()
	paint.DropShadow(c, geom.RectXYWH(40, 20, 60, 30), 6, geom.Pt{Y: 3}, 8, paint.Color{A: 0.6})
}

func render(t *testing.T, draw func(paint.Canvas)) []byte {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	c := p.BeginOffscreen(geom.Size{W: 200, H: 160}, 2)
	draw(c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, p.Image()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReplayMatchesDirect(t *testing.T) {
	direct := render(t, drawScene)

	var list scene.List
	drawScene(list.Recorder())
	if list.Len() == 0 {
		t.Fatal("nothing recorded")
	}
	replayed := render(t, list.Replay)

	if !bytes.Equal(direct, replayed) {
		t.Fatal("record+replay must be pixel-identical to direct painting")
	}
}

func TestResetKeepsWorking(t *testing.T) {
	var list scene.List
	drawScene(list.Recorder())
	n := list.Len()
	list.Reset()
	if list.Len() != 0 {
		t.Fatal("reset should clear ops")
	}
	drawScene(list.Recorder())
	if list.Len() != n {
		t.Fatalf("re-record op count = %d, want %d", list.Len(), n)
	}
}

func drawTransformed(c paint.Canvas) {
	c.Clear(paint.RGB(0.1, 0.1, 0.12))
	c.PushTransform(paint.MapRect(geom.RectXYWH(0, 0, 40, 20), geom.RectXYWH(60, 60, 80, 40)))
	c.FillRRect(geom.RectXYWH(0, 0, 40, 20), 4, paint.RGB(0.9, 0.5, 0.2))
	c.TextIn("", "hi", geom.Pt{X: 4, Y: 15}, 12, paint.RGB(1, 1, 1))
	c.PopTransform()
}

func TestTransformReplayMatchesDirect(t *testing.T) {
	direct := render(t, drawTransformed)

	var list scene.List
	drawTransformed(list.Recorder())
	if !list.HasLayers() {
		t.Fatal("a recorded transform must set HasLayers (full-repaint frame)")
	}
	replayed := render(t, list.Replay)
	if !bytes.Equal(direct, replayed) {
		t.Fatal("transform record+replay must be pixel-identical to direct painting")
	}
}

var testImage = func() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 3)
	}
	return img
}()
