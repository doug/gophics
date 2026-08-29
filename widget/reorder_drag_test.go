package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Dragging a row to a new position must actually reorder.
//
// OnPressEnd fires when a press concludes "for any reason", and one of those
// reasons is a drag committing — which the host reports to every box that took
// the press, the winner included. Reorderable used OnPressEnd to end its drag,
// so the drag was ended by the very move that started it: the row was dropped
// back where it began before it had travelled anywhere, and no reorder was
// ever reported.
func TestDraggingARowReordersIt(t *testing.T) {
	const rowH = 60
	var gotFrom, gotTo = -1, -1
	root := widget.Reorderable{
		Count:      5,
		ItemExtent: rowH,
		Build: func(i int) widget.Widget {
			return widget.Sized{H: rowH, Child: widget.Decorated{Color: paint.RGB(0.5, 0.5, 0.5)}}
		},
		OnReorder: func(from, to int) { gotFrom, gotTo = from, to },
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 400}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Drag row 0 (centre y=30) down past two rows, to y=150 — row 2's band.
	h.Drag(geom.Pt{X: 150, Y: 30}, geom.Pt{X: 150, Y: 150})
	h.Render()

	if gotFrom < 0 {
		t.Fatal("dragging a row reported no reorder at all")
	}
	if gotFrom != 0 {
		t.Errorf("reordered from row %d, want 0", gotFrom)
	}
	if gotTo == 0 {
		t.Errorf("row landed back at 0; the drag moved it %d rows", gotTo)
	}
}

// A widget that drags still hears that its press ended, once the gesture is
// over. Holding the drag winner back at commit time must defer that call, not
// discard it: a handler that raises a pressed-state in OnPress and lowers it in
// OnPressEnd would otherwise stay lit for good after any drag.
func TestADragWinnerIsStillToldItsPressEnded(t *testing.T) {
	var pressed, ended int
	root := widget.Interactive{
		Gestures: widget.Gestures{
			OnPress:    func(geom.Pt) { pressed++ },
			OnDrag:     func(geom.Pt, geom.Pt) {},
			OnPressEnd: func() { ended++ },
		},
		Child: widget.Sized{W: 200, H: 200, Child: widget.Decorated{Color: paint.RGB(0.5, 0.5, 0.5)}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 300}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	h.Drag(geom.Pt{X: 50, Y: 50}, geom.Pt{X: 150, Y: 150})
	h.Render()

	if pressed != 1 {
		t.Fatalf("OnPress fired %d times, want 1", pressed)
	}
	if ended != 1 {
		t.Errorf("OnPressEnd fired %d times after a drag, want exactly 1", ended)
	}
}

// The gallery's arrangement: a reorderable list inside a scrolling page.
//
// A row drags along the same axis the page scrolls, so the two compete for
// every movement. The row is the deeper candidate and has to win, or a list
// cannot be reordered at all on a page that scrolls — which is every page it
// is likely to appear on.
func TestARowReordersInsideAScrollingPage(t *testing.T) {
	const rowH = 60
	var gotFrom, gotTo = -1, -1
	list := widget.Reorderable{
		Count:      5,
		ItemExtent: rowH,
		Build: func(i int) widget.Widget {
			return widget.Sized{H: rowH, Child: widget.Decorated{Color: paint.RGB(0.5, 0.5, 0.5)}}
		},
		OnReorder: func(from, to int) { gotFrom, gotTo = from, to },
	}
	root := widget.Scroll{Axis: layout.Vertical, Child: widget.Column(
		widget.Sized{H: rowH * 5, Child: list},
		widget.Sized{H: 600},
	)}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 400}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	h.TouchDrag(geom.Pt{X: 150, Y: 30}, geom.Pt{X: 150, Y: 150})
	h.Render()

	if gotFrom < 0 {
		t.Fatal("inside a scroll, dragging a row reported no reorder at all")
	}
	if gotTo == 0 {
		t.Errorf("row landed back at 0; the scroll took the gesture")
	}
}
