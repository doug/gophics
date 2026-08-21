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

// A glass surface whose backdrop is the page's clear colour, not a painted
// widget.
//
// The GPU blur builds its backdrop by re-rendering the draws queued before it
// into an offscreen texture. Config.Background is not a draw — it is the clear
// colour of the frame — so anywhere the UI has not painted real geometry, that
// offscreen is transparent, and blurring transparency gives black. Which is
// exactly "a black cover over it".
//
// The earlier glass test passed because it painted its backdrop with an
// explicit Fill widget, which *is* a draw command.
func TestGPUGlassOverClearBackgroundIsNotBlack(t *testing.T) {
	bg := paint.RGB(0.95, 0.75, 0.2) // bright, so black is unmistakable

	h, err := NewHeadless(
		widget.Provide[theme.Theme]{Value: theme.Glass(),
			Child: widget.Padding{All: 30, Child: theme.Card{Pad: 20,
				Child: widget.Sized{W: 120, H: 60}}}},
		Config{Size: geom.Size{W: 220, H: 140}, Background: bg, Font: goregular.TTF}, 1)
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
	t.Logf("glass over CLEAR background: cpu=(%.0f,%.0f,%.0f)  gpu=(%.0f,%.0f,%.0f)",
		cr, cg, cb, gr, gg, gb)

	if gr+gg+gb < 150 {
		t.Errorf("the glass panel is near-black on the GPU (%.0f,%.0f,%.0f) while the "+
			"CPU renders it (%.0f,%.0f,%.0f) — the blur's backdrop offscreen does not "+
			"contain the page's clear colour", gr, gg, gb, cr, cg, cb)
	}
}
