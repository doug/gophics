//go:build gophics_gpu

package app

import (
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A shape drawn after text must cover it.
//
// A ScissorGroup buckets its commands by type and renders the buckets in a
// fixed order — shapes, then paths, then images, then text — so text and a
// shape sharing one group are drawn in bucket order rather than queue order,
// and the text lands on top however early it was queued. In solitaire that put
// the pips of a covered card over the card covering it.
func TestGPUShapeAfterTextCoversIt(t *testing.T) {
	// Text first, then an opaque panel over the top of it. Same clip
	// throughout, so without a bucket split both land in one group.
	body := widget.Stack{Children: []widget.Widget{
		widget.Fill{Color: paint.RGB(1, 1, 1)},
		widget.Padding{All: 10, Child: widget.Text{
			Value: "HIDDEN", Size: 30, Color: paint.RGB(0.85, 0, 0),
		}},
		widget.Fill{Color: paint.RGB(0, 0.35, 0.15)}, // covers everything
	}}

	h, err := NewHeadless(body, Config{
		Size: geom.Size{W: 200, H: 80}, Background: paint.RGB(1, 1, 1),
		Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}

	cpu := toRGBA(h.Render())
	gpuImg := h.RenderGPU()
	if gpuImg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu := toRGBA(gpuImg)

	// Count pixels carrying the text's red, which must be none: the panel is
	// opaque and drawn last.
	redish := func(img interface{ RGBAAt(x, y int) color.RGBA }) int {
		n := 0
		for y := 0; y < 80; y++ {
			for x := 0; x < 200; x++ {
				p := img.RGBAAt(x, y)
				if p.R > 120 && p.G < 90 && p.B < 90 {
					n++
				}
			}
		}
		return n
	}

	c, g := redish(cpu), redish(gpu)
	t.Logf("text pixels showing through: cpu=%d gpu=%d", c, g)

	if c != 0 {
		t.Fatalf("the CPU rasterizer also leaks the text (%d px); the test setup "+
			"does not actually cover it", c)
	}
	if g != 0 {
		t.Errorf("%d text pixels show through an opaque shape drawn after them on "+
			"the GPU — buckets are being rendered out of queue order", g)
	}
}
