package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type retainApp struct{ hook func(*retainAppState) }

func (a retainApp) CreateState() widget.State { s := &retainAppState{}; s.hook = a.hook; return s }

type retainAppState struct {
	widget.StateBase[retainApp]
	hook func(*retainAppState)
	step int
}

func (s *retainAppState) Init(widget.Ctx) { s.hook(s) }

func (s *retainAppState) Build(widget.Ctx) widget.Widget {
	colors := []paint.Color{paint.RGB(0, 0, 0), paint.RGB(1, 1, 1), paint.RGB(1, 0, 0)}
	return widget.Fill{Color: colors[s.step]}
}

// Headless.Render's contract is that the image it returns is retained until
// the next changed frame (see its doc comment); a caller may hold that
// reference across several further Render calls before it looks at the
// pixels. TestRetainedImageSurvivesLaterRenders pins that: a backing pixmap
// reused for a later frame is a correctness bug even though it is invisible
// to every test that just checks the current frame.
func TestRetainedImageSurvivesLaterRenders(t *testing.T) {
	var st *retainAppState
	h, err := NewHeadless(retainApp{hook: func(s *retainAppState) { st = s }},
		Config{Size: geom.Size{W: 8, H: 8}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}

	img0 := toRGBARetainTest(h.Render()) // frame 0: black
	saved := img0.RGBAAt(1, 1)

	st.step = 1
	h.core.Owner.RebuildAll()
	h.Render() // frame 1: white

	st.step = 2
	h.core.Owner.RebuildAll()
	h.Render() // frame 2: red

	after := img0.RGBAAt(1, 1)
	if after != saved {
		t.Fatalf("img0 mutated after being retained across later renders: was %+v, now %+v (aliased backing buffer)", saved, after)
	}
}

func toRGBARetainTest(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}
