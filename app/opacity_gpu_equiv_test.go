//go:build gophics_gpu

package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A group at partial opacity must composite the same on the GPU as on the CPU.
//
// Reported from a phone: the gallery's card fades to nothing rather than to the
// 0.25 it asks for. The CPU path is exactly right (0.25 black over white gives
// 191), so a difference here is the GPU's group-opacity composite.
func TestGPUOpacityGroupMatchesCPU(t *testing.T) {
	for _, alpha := range []float32{1, 0.5, 0.25} {
		h, err := NewHeadless(widget.Opacity{
			Alpha: alpha,
			Child: widget.Fill{Color: paint.RGB(0, 0, 0), Child: widget.Sized{W: 60, H: 60}},
		}, Config{Size: geom.Size{W: 60, H: 60}, Background: paint.RGB(1, 1, 1)}, 1)
		if err != nil {
			t.Fatal(err)
		}

		cpu := toRGBA(h.Render())
		gpuImg := h.RenderGPU()
		if gpuImg == nil {
			t.Skip("no headless GPU adapter")
		}
		gpu := toRGBA(gpuImg)

		cx, cy := cpu.Bounds().Dx()/2, cpu.Bounds().Dy()/2
		c := cpu.RGBAAt(cx, cy)
		g := gpu.RGBAAt(cx, cy)

		t.Logf("alpha=%.2f  cpu=(%d,%d,%d)  gpu=(%d,%d,%d)", alpha, c.R, c.G, c.B, g.R, g.G, g.B)

		diff := int(c.R) - int(g.R)
		if diff < 0 {
			diff = -diff
		}
		if diff > 8 {
			t.Errorf("alpha=%.2f: cpu r=%d but gpu r=%d — the group alpha is not "+
				"composited the same on the GPU", alpha, c.R, g.R)
		}
	}
}
