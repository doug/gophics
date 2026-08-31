//go:build gophics_gpu

package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A backdrop blur really frosts, on both renderers.
//
// The blur has three possible fates and two of them are silent. Without a GPU
// context it box-blurs the pixmap in place; with one it queues a command the
// resolve pass turns into a reduced-resolution offscreen composite; and on the
// rasterAtlas strategy resolveLayers strips the command entirely, because there
// is no layer target to render the backdrop into. Nothing fails in that last
// case — the card still draws, just flat — so no existing test would notice a
// frost that quietly stopped happening.
//
// Hence an assertion on the pixels rather than on the mechanism: a hard
// black/white edge behind a translucent card has to come out as a run of
// intermediate values under the card, because that is what blurring an edge
// does. A dropped blur leaves two levels and nothing in between.
func blurSeam(t *testing.T, gpu bool) []int {
	t.Helper()
	const w, h = 200, 120
	root := widget.Fill{Color: paint.RGB(1, 1, 1), Child: widget.Stack{Children: []widget.Widget{
		widget.Align{X: 0, Y: 0, Child: widget.Sized{W: w / 2, H: h,
			Child: widget.Decorated{Color: paint.RGB(0, 0, 0)}}},
		widget.Padding{All: 20, Child: widget.Decorated{
			Color: paint.Color{R: 1, G: 1, B: 1, A: 0.35}, Radius: 8, Blur: 20,
			// Decorated has no intrinsic size: with no child it collapses to
			// nothing and paints no card at all.
			Child: widget.Sized{W: w - 40, H: h - 40},
		}},
	}}}
	hl, err := NewHeadless(root, Config{Size: geom.Size{W: w, H: h}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	var img image.Image
	if gpu {
		if img = hl.RenderGPU(); img == nil {
			t.Skip("no GPU adapter available")
		}
		// A software adapter is an adapter, so the nil check above lets it
		// through — and it does not blur. Measured on the UTM Windows VM, whose
		// "Software Renderer" backend produces a hard step across the seam
		// (89..89, 245, 255..255) where a blur produces a ramp. That made the
		// whole app suite red on any machine without a GPU driver, which is a
		// broken gate rather than a caught defect: blur is a GPU feature and
		// this test asserts it works, not that every adapter has it.
		if softwareAdapter() {
			t.Skip("software adapter does not implement backdrop blur")
		}
	} else {
		img = hl.Render()
	}
	// Walk across the seam, inside the card.
	var out []int
	for x := w/2 - 14; x <= w/2+14; x += 2 {
		r, _, _, _ := img.At(x, h/2).RGBA()
		out = append(out, int(r>>8))
	}
	return out
}

func TestBackdropBlurFrostsOnBothRenderers(t *testing.T) {
	for _, tc := range []struct {
		name string
		gpu  bool
	}{{"cpu", false}, {"gpu", true}} {
		t.Run(tc.name, func(t *testing.T) {
			seam := blurSeam(t, tc.gpu)
			// A blurred edge climbs; a sharp one is two flat runs.
			levels := 1
			for i := 1; i < len(seam); i++ {
				if seam[i]-seam[i-1] > 2 {
					levels++
				}
			}
			if levels < 8 {
				t.Errorf("only %d rising steps across the seam (%v): the backdrop was not blurred", levels, seam)
			}
			if seam[0] > 160 || seam[len(seam)-1] < 190 {
				t.Errorf("seam %v does not run dark to light; the card is not over the edge", seam)
			}
		})
	}
}

// The two renderers are meant to produce the same frost — the GPU kernel is
// sized to match the CPU path's 3-pass box blur. They are not bit-identical
// and are not required to be, but a divergence wide enough to see is a bug in
// one of them.
func TestBothRenderersFrostAlike(t *testing.T) {
	cpu := blurSeam(t, false)
	gpu := blurSeam(t, true)
	for i := range cpu {
		if d := cpu[i] - gpu[i]; d > 24 || d < -24 {
			t.Errorf("sample %d differs by %d (cpu %v, gpu %v)", i, d, cpu, gpu)
		}
	}
}
