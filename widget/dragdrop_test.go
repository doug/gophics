package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// dnd holds the observable results of a drag.
type dnd struct {
	dropped  []string
	starts   int
	ends     []bool
	hovering map[string]bool
}

// dndApp stacks a 40pt draggable source, then a target that refuses strings,
// then one that accepts them — so a drag can be walked across both.
type dndApp struct{ r *dnd }

func (a dndApp) Build(widget.Ctx) widget.Widget {
	r := a.r
	source := widget.Draggable{
		Payload:     "card-1",
		OnDragStart: func() { r.starts++ },
		OnDragEnd:   func(ok bool) { r.ends = append(r.ends, ok) },
		Child:       widget.Sized{W: 200, H: 40},
	}
	refusing := widget.DropTarget{
		Accept: func(p any) bool { _, ok := p.(int); return ok },
		OnDrop: func(any, geom.Pt) { r.dropped = append(r.dropped, "WRONG") },
		Builder: func(h bool) widget.Widget {
			r.hovering["refuse"] = h
			return widget.Sized{W: 200, H: 40}
		},
	}
	accepting := widget.DropTarget{
		Accept: func(p any) bool { _, ok := p.(string); return ok },
		OnDrop: func(p any, _ geom.Pt) { r.dropped = append(r.dropped, p.(string)) },
		Builder: func(h bool) widget.Widget {
			r.hovering["accept"] = h
			return widget.Sized{W: 200, H: 40}
		},
	}
	return widget.Column(source, refusing, accepting)
}

// Source occupies y 0..40, the refusing target 40..80, the accepting one
// 80..120.
const (
	srcY    = 20
	refuseY = 60
	acceptY = 100
)

func newDnD(t *testing.T) (*app.Headless, *dnd) {
	t.Helper()
	r := &dnd{hovering: map[string]bool{}}
	h, err := app.NewHeadless(dndApp{r: r}, app.Config{
		Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, r
}

func TestDragDropDeliversPayload(t *testing.T) {
	h, r := newDnD(t)
	h.Drag(geom.Pt{X: 100, Y: srcY}, geom.Pt{X: 100, Y: acceptY})
	h.Render()

	if r.starts != 1 {
		t.Errorf("OnDragStart fired %d times, want 1", r.starts)
	}
	if len(r.dropped) != 1 || r.dropped[0] != "card-1" {
		t.Fatalf("dropped = %v, want [card-1]", r.dropped)
	}
	if len(r.ends) != 1 || !r.ends[0] {
		t.Errorf("OnDragEnd = %v, want [true]", r.ends)
	}
}

// A target whose Accept rejects the payload must neither highlight nor
// receive it, even when the pointer is released squarely on it.
func TestDropTargetRefusesWrongPayload(t *testing.T) {
	h, r := newDnD(t)
	h.Drag(geom.Pt{X: 100, Y: srcY}, geom.Pt{X: 100, Y: refuseY})
	h.Render()

	if len(r.dropped) != 0 {
		t.Errorf("dropped = %v, want nothing delivered", r.dropped)
	}
	if r.hovering["refuse"] {
		t.Error("refusing target highlighted for a payload it does not accept")
	}
	// The gesture still ended, and it reports that nothing took the payload.
	if len(r.ends) != 1 || r.ends[0] {
		t.Errorf("OnDragEnd = %v, want [false]", r.ends)
	}
}

// Hover feedback has to track the pointer while the drag is in flight, not
// only at the moment of release.
func TestDropTargetHoverTracksDrag(t *testing.T) {
	h, r := newDnD(t)

	h.DragTo(geom.Pt{X: 100, Y: srcY}, geom.Pt{X: 100, Y: acceptY})
	h.Render()
	if !r.hovering["accept"] {
		t.Error("accepting target is not highlighted mid-drag")
	}
	if r.hovering["refuse"] {
		t.Error("refusing target highlighted mid-drag")
	}

	// Moving off it clears the highlight again. This is a Move, not another
	// DragTo: the drag is one continuous gesture and must not be re-pressed.
	h.Move(geom.Pt{X: 100, Y: refuseY})
	h.Render()
	if r.hovering["accept"] {
		t.Error("highlight stuck on after the pointer left")
	}
	h.Release(geom.Pt{X: 100, Y: refuseY})
}

// A press that never travels is a tap, not a drag: nothing should be picked up.
func TestTapDoesNotStartDrag(t *testing.T) {
	h, r := newDnD(t)
	h.Tap(geom.Pt{X: 100, Y: srcY})
	h.Render()
	if r.starts != 0 {
		t.Errorf("OnDragStart fired %d times on a tap, want 0", r.starts)
	}
	if len(r.dropped) != 0 {
		t.Errorf("dropped = %v on a tap, want nothing", r.dropped)
	}
}

// Releasing over empty space cancels cleanly and reports the miss.
func TestDropOnNothing(t *testing.T) {
	h, r := newDnD(t)
	h.Drag(geom.Pt{X: 100, Y: srcY}, geom.Pt{X: 100, Y: 180})
	h.Render()
	if len(r.dropped) != 0 {
		t.Errorf("dropped = %v, want nothing", r.dropped)
	}
	if len(r.ends) != 1 || r.ends[0] {
		t.Errorf("OnDragEnd = %v, want [false]", r.ends)
	}
}
