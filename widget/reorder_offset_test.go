package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// The row being dragged stays under the pointer.
//
// The list shows the dragged row in the slot it would land in, which is what
// opens the gap ahead of it — so by the time the finger has passed one row,
// the row has already been moved a whole ItemExtent by the reordering itself.
// Adding the full pointer delta on top of that made it travel twice: it ran
// away downward, gaining another row's height every time it crossed one.
func TestTheDraggedRowTracksThePointer(t *testing.T) {
	const rowH = 60
	label := func(i int) string { return string(rune('A' + i)) }
	root := widget.Reorderable{
		Count:      5,
		ItemExtent: rowH,
		Build: func(i int) widget.Widget {
			return widget.Sized{H: rowH, Child: widget.Semantics{
				Label: label(i), Child: widget.Decorated{Color: paint.RGB(0.5, 0.5, 0.5)}}}
		},
		OnReorder: func(int, int) {},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 400}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	midOf := func(name string) float32 {
		for _, n := range h.Semantics() {
			if n.Label == name {
				return (n.Rect.Min.Y + n.Rect.Max.Y) / 2
			}
		}
		t.Fatalf("no row %q", name)
		return 0
	}
	start := midOf("A")

	// Walk the pointer down past three rows, checking at every step that the
	// row has moved exactly as far as the pointer has.
	h.Press(geom.Pt{X: 150, Y: start})
	for _, dy := range []float32{20, 50, 90, 130, 170} {
		h.Move(geom.Pt{X: 150, Y: start + dy})
		h.Render()
		if got, want := midOf("A")-start, dy; got < want-1 || got > want+1 {
			t.Errorf("pointer moved %.0fpt but the row moved %.0fpt", want, got)
		}
	}
	h.Release(geom.Pt{X: 150, Y: start + 170})
}
