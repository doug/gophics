package apptest_test

import (
	"testing"
	"time"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// LayoutObserver reports the child's laid-out rect in root space, once per
// change: after a resize that moves it, and not for a frame that merely
// repaints it where it was.
func TestLayoutObserverReportsRectAndResize(t *testing.T) {
	var got []geom.Rect
	root := widget.Column(
		widget.Sized{W: 50, H: 20},
		widget.Padding{All: 10, Child: widget.LayoutObserver{
			OnLayout: func(r geom.Rect) { got = append(got, r) },
			// A row with an expanding child takes the full width it is given,
			// so a resize changes the rect and the observer must speak again.
			Child: widget.Row(widget.Expand(widget.Sized{H: 30})),
		}},
	)
	a := apptest.New(t, root, apptest.Size(200, 100))

	// The frame that paints measures; the callback is posted and runs at the
	// top of the next one, the same one-frame settle LayoutBuilder has.
	a.Render()
	a.Render()
	want := geom.Rect{Min: geom.Pt{X: 10, Y: 30}, Max: geom.Pt{X: 190, Y: 60}}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("after first frame: reports %v, want exactly [%v]", got, want)
	}

	// Unchanged frames are silent.
	a.Render()
	a.Render()
	if len(got) != 1 {
		t.Fatalf("repainting an unmoved child reported again: %v", got)
	}

	a.Resize(geom.Size{W: 300, H: 100})
	a.Render()
	a.Render()
	want2 := geom.Rect{Min: geom.Pt{X: 10, Y: 30}, Max: geom.Pt{X: 290, Y: 60}}
	if len(got) != 2 || got[1] != want2 {
		t.Fatalf("after resize: reports %v, want second report %v", got, want2)
	}
}

// The origin follows the scroll offset: it is where the child was painted,
// which is what a drop-point comparison in root space needs.
func TestLayoutObserverOriginTracksScroll(t *testing.T) {
	var got []geom.Rect
	items := make([]widget.Widget, 20)
	for i := range items {
		items[i] = widget.Sized{W: 100, H: 40}
	}
	// Item 3, not one at the fold: a child wholly outside the viewport is
	// culled — never painted — and so has nothing to report, by design.
	items[3] = widget.LayoutObserver{
		OnLayout: func(r geom.Rect) { got = append(got, r) },
		Child:    widget.Sized{W: 100, H: 40},
	}
	a := apptest.New(t, widget.Scroll{Child: widget.Column(items...)}, apptest.Size(200, 200))
	a.Render()
	a.Render()
	if len(got) != 1 || got[0].Min.Y != 120 {
		t.Fatalf("item 3 should sit at y=120 before scrolling, reports %v", got)
	}
	// Over the 100px-wide content: a Scroll's wheel target is its viewport,
	// which shrink-wraps to the column here.
	a.ScrollAt(geom.Pt{X: 50, Y: 100}, geom.Pt{Y: -100})
	a.Render()
	a.Render()
	if len(got) != 2 || got[1].Min.Y != 20 {
		t.Fatalf("after scrolling 100px, item 3 should report y=20, reports %v", got)
	}
}

// Every observer in a frame reports, however many there are. Reports are
// posted from Paint on the UI goroutine, and the post queue used to be a
// bounded channel drained only by that same goroutine: a first frame with
// more observers than the channel held blocked forever. A board with an
// observer per row is exactly that frame.
func TestManyLayoutObserversReportWithoutBlocking(t *testing.T) {
	const n = 300
	reported := make([]bool, n)
	rows := make([]widget.Widget, n)
	for i := range rows {
		i := i
		rows[i] = widget.LayoutObserver{
			OnLayout: func(geom.Rect) { reported[i] = true },
			Child:    widget.Sized{W: 100, H: 1},
		}
	}
	// No Scroll: nothing is culled, so all n paint in the first frame.
	a := apptest.New(t, widget.Column(rows...), apptest.Size(200, 400))

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Render() // paints and posts n reports
		a.Render() // drains them
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("rendering a frame with 300 LayoutObservers hung: the post queue blocked the UI goroutine")
	}
	missing := 0
	for _, r := range reported {
		if !r {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("%d of %d observers never reported", missing, n)
	}
}
