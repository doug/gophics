//go:build gophics_gpu

package app

import (
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// A glass surface is a translucent panel over a backdrop blur. On the GPU that
// blur has to sample what is already on the framebuffer; sampling something
// unresolved or uncleared yields a dark rectangle, which is what "a black cover
// over it" describes.
//
// The panel sits over a bright backdrop, so a correct blur stays bright.
func TestGPUGlassSurfaceIsNotBlack(t *testing.T) {
	th := theme.Glass()
	body := widget.Provide[theme.Theme]{Value: th, Child: widget.Stack{Children: []widget.Widget{
		// A bright backdrop for the blur to pick up.
		widget.Fill{Color: paint.RGB(0.95, 0.75, 0.2)},
		widget.Padding{All: 30, Child: theme.Card{Pad: 20,
			Child: widget.Sized{W: 120, H: 60}}},
	}}}

	h, err := NewHeadless(body, Config{
		Size: geom.Size{W: 220, H: 140}, Background: paint.RGB(0.95, 0.75, 0.2),
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

	mean := func(img interface{ RGBAAt(x, y int) color.RGBA }) (r, g, b float64) {
		var n float64
		for y := 55; y < 85; y++ {
			for x := 60; x < 160; x++ {
				p := img.RGBAAt(x, y)
				r += float64(p.R)
				g += float64(p.G)
				b += float64(p.B)
				n++
			}
		}
		return r / n, g / n, b / n
	}

	cr, cg, cb := mean(cpu)
	gr, gg, gb := mean(gpu)
	t.Logf("glass panel interior: cpu=(%.0f,%.0f,%.0f)  gpu=(%.0f,%.0f,%.0f)", cr, cg, cb, gr, gg, gb)

	if gr+gg+gb < 120 {
		t.Errorf("the glass panel renders near-black on the GPU (%.0f,%.0f,%.0f) — "+
			"the backdrop blur is sampling nothing", gr, gg, gb)
	}
	for _, d := range []struct {
		name string
		c, g float64
	}{{"R", cr, gr}, {"G", cg, gg}, {"B", cb, gb}} {
		if diff := d.c - d.g; diff > 40 || diff < -40 {
			t.Errorf("glass %s: cpu=%.0f gpu=%.0f — the blurred surface differs by %.0f",
				d.name, d.c, d.g, diff)
		}
	}
}
