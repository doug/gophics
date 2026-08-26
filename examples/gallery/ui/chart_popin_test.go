//go:build gophics_gpu

package ui_test

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
)

// A chart scrolling into view is revealed, not switched on.
//
// Reported as "the whole chart pops in rather than being properly clipped",
// and never reproduced. It matters more since Canvas{Clip: true}: an unclipped
// Canvas reports Unbounded ink and can never be culled, so before that change
// a chart was always painted whatever its position. Now it has real bounds and
// a container may cull it — correct while it is off-screen, and a pop-in if
// the test for "off-screen" is wrong at the boundary.
//
// So: walk a chart card up through the viewport edge a few pixels at a time
// and count how much of it is drawn. A reveal grows; a pop-in goes from
// nothing to everything between two adjacent frames.
func TestAChartIsRevealedNotSwitchedOn(t *testing.T) {
	out := os.Getenv("POPIN_OUT")
	a := apptest.New(t, ui.Gallery{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
	}))
	if a.RenderGPU() == nil {
		t.Skip("no GPU adapter available")
	}
	settle := func() {
		for s := 0; s < 40; s++ {
			a.Step(1.0 / 60)
		}
	}
	const section = "Charts"
	a.Move(geom.Pt{X: 210, Y: 400})
	for i := 0; i < 20 && a.NodeContaining(section).Rect.Min.Y > 640; i++ {
		a.Scroll(geom.Pt{Y: -400})
		settle()
	}
	a.TapText(section)
	settle()

	// Ink below the header band, which is where a card crossing the top edge
	// appears from. Counting non-background pixels is enough: the question is
	// how much of the chart is on screen, not what it looks like.
	inked := func(img image.Image) int {
		b := img.Bounds()
		n := 0
		for y := b.Min.Y + 300; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x += 3 {
				r, g, bl, _ := img.At(x, y).RGBA()
				if r>>8 < 240 || g>>8 < 240 || bl>>8 < 240 {
					n++
				}
			}
		}
		return n
	}

	var counts []int
	for step := 0; step < 14; step++ {
		a.Scroll(geom.Pt{Y: -40})
		settle()
		img := a.RenderGPU()
		counts = append(counts, inked(img))
		if out != "" {
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("popin_%02d.png", step)))
			if err == nil {
				png.Encode(f, img)
				f.Close()
			}
		}
	}

	// A pop-in is a step where almost nothing becomes almost everything. With
	// 40pt of scroll per step, no single step should add most of the ink.
	for i := 1; i < len(counts); i++ {
		prev, cur := counts[i-1], counts[i]
		if prev < 200 && cur > 4000 {
			t.Errorf("step %d went from %d inked pixels to %d — that is a pop-in, not a reveal",
				i, prev, cur)
		}
	}
	t.Logf("ink per step: %v", counts)
}
