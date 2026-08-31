package widget_test

import (
	"image"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"golang.org/x/image/font/gofont/goregular"
)

// The drag preview must not leave a trail.
//
// Reported as "it doesn't seem like it is clearing the buffer": dragging a chip
// smeared a copy of it at every position the pointer had passed through, each
// alpha-blended over the last, so the whole path filled in. The preview lives
// in the overlay above the DragHost, so what it costs is a repaint of the
// region it vacates — if damage covers only where the preview now is, then
// everywhere it has been keeps its pixels.
//
// TestTheDragGhostSitsUnderThePointer covers where the preview is; nothing
// covered what happens to where it was.
func TestDragPreviewLeavesNoTrail(t *testing.T) {
	h, err := app.NewHeadless(ghostApp{}, app.Config{
		Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	const px, py = 150, 150 // a point the drag passes through and then leaves
	before := pixelAt(h.Render(), px, py)

	h.Press(geom.Pt{X: 150, Y: 20})
	h.Move(geom.Pt{X: px, Y: py})
	h.Render()
	if over := pixelAt(h.Render(), px, py); over == before {
		t.Skip("the preview never covered the probe; reposition it")
	}

	h.Move(geom.Pt{X: 150, Y: 260}) // well clear of the probe
	h.Render()
	after := pixelAt(h.Render(), px, py)

	h.Release(geom.Pt{X: 150, Y: 260})
	h.Render()

	if after != before {
		t.Errorf("the vacated point is %v but was %v before the drag — the preview "+
			"left a copy of itself where it had been", after, before)
	}
}

func pixelAt(img image.Image, x, y int) [4]uint32 {
	r, g, b, a := img.At(x, y).RGBA()
	return [4]uint32{r, g, b, a}
}
