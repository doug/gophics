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

// Text must land with the same weight on the GPU as on the CPU.
//
// Measured while chasing a card that vanished at low opacity: the card's own
// surface turned out to be white on off-white — six levels of contrast — so the
// only thing carrying the demo was the text inside it, and that appeared to
// render lighter on the GPU. If it does, every dimmed or low-contrast text in
// the app is fainter there, and anything relying on text for contrast
// disappears first.
func TestGPUTextInkMatchesCPU(t *testing.T) {
	h, err := NewHeadless(
		widget.Fill{Color: paint.RGB(1, 1, 1), Child: widget.Padding{All: 10,
			Child: widget.Text{S: "Grouped opacity", Size: 18, Color: paint.RGB(0, 0, 0)}}},
		Config{Size: geom.Size{W: 200, H: 60}, Background: paint.RGB(1, 1, 1),
			Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}

	cpu := toRGBA(h.Render())
	gpuImg := h.RenderGPU()
	if gpuImg == nil {
		t.Skip("no headless GPU adapter")
	}
	gpu := toRGBA(gpuImg)

	// Total darkness laid down across the whole image: how much ink the text is.
	ink := func(img interface{ RGBAAt(x, y int) color.RGBA }) float64 {
		var sum float64
		for y := 0; y < 60; y++ {
			for x := 0; x < 200; x++ {
				sum += 255 - float64(img.RGBAAt(x, y).R)
			}
		}
		return sum
	}

	c, g := ink(cpu), ink(gpu)
	t.Logf("text ink: cpu=%.0f  gpu=%.0f  ratio=%.2f", c, g, g/c)

	if c == 0 {
		t.Fatal("no text rendered on the CPU; the test measures nothing")
	}
	if r := g / c; r < 0.8 || r > 1.25 {
		t.Errorf("the GPU lays down %.0f%% of the CPU's text ink — text is a "+
			"different weight there, and low-contrast text disappears first", r*100)
	}
}
