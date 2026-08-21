//go:build gophics_gpu

package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A colour that carries its own alpha must composite the same on the GPU as on
// the CPU.
//
// Every opacity test that passed used a fully opaque colour, where premultiplied
// and straight alpha are identical — so none of them could see this. A theme
// surface is translucent, and a glass theme is nothing but translucent
// surfaces; "a black cover over it" is what a white translucent fill looks like
// when its alpha is applied to the colour twice.
func TestGPUTranslucentFillMatchesCPU(t *testing.T) {
	// White at half alpha over a white page should stay light. Premultiplied
	// twice it goes grey; treated as straight when it is premultiplied it goes
	// dark.
	cases := []struct {
		name string
		col  paint.Color
	}{
		{"white a=0.5", paint.Color{R: 1, G: 1, B: 1, A: 0.5}},
		{"white a=0.25", paint.Color{R: 1, G: 1, B: 1, A: 0.25}},
		{"black a=0.5", paint.Color{R: 0, G: 0, B: 0, A: 0.5}},
		{"blue a=0.5", paint.Color{R: 0.2, G: 0.4, B: 0.9, A: 0.5}},
	}

	for _, tc := range cases {
		h, err := NewHeadless(
			widget.Fill{Color: tc.col, Child: widget.Sized{W: 80, H: 80}},
			Config{Size: geom.Size{W: 80, H: 80}, Background: paint.RGB(1, 1, 1)}, 1)
		if err != nil {
			t.Fatal(err)
		}

		cpu := toRGBA(h.Render())
		gpuImg := h.RenderGPU()
		if gpuImg == nil {
			t.Skip("no headless GPU adapter")
		}
		gpu := toRGBA(gpuImg)

		c := cpu.RGBAAt(40, 40)
		g := gpu.RGBAAt(40, 40)
		t.Logf("%-14s cpu=(%3d,%3d,%3d)  gpu=(%3d,%3d,%3d)", tc.name, c.R, c.G, c.B, g.R, g.G, g.B)

		for _, ch := range []struct {
			name string
			a, b uint8
		}{{"R", c.R, g.R}, {"G", c.G, g.G}, {"B", c.B, g.B}} {
			d := int(ch.a) - int(ch.b)
			if d < 0 {
				d = -d
			}
			if d > 8 {
				t.Errorf("%s: %s channel cpu=%d gpu=%d (off by %d) — a colour "+
					"carrying alpha is not composited the same on the GPU",
					tc.name, ch.name, ch.a, ch.b, d)
			}
		}
	}
}
