package widget_test

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// A DropTarget scrolled out of view is not there to drop on.
//
// A target records its root-space rect in its own Paint, and Flex culls the
// paint of children outside the clip — so a target scrolled off screen kept
// the rect from the last frame it was visible in, and a payload carried over
// that patch of screen hovered and dropped onto a target the user could not
// see. The DragHost's box now clears every rect before the frame paints.
func TestCulledDropTargetDoesNotMatch(t *testing.T) {
	var hover []bool
	dropped := 0
	root := widget.Scroll{Child: widget.Column(
		widget.DropTarget{
			Builder: func(hv bool) widget.Widget {
				hover = append(hover, hv)
				return widget.Sized{W: 300, H: 100}
			},
			OnDrop: func(any, geom.Pt) { dropped++ },
		},
		widget.Sized{W: 300, H: 2000},
		widget.Draggable{Payload: 1, Child: widget.Sized{W: 300, H: 40}},
	)}
	h := headless(t, root, 320, 240)
	h.Render()
	h.Move(geom.Pt{X: 160, Y: 120})
	h.Scroll(geom.Pt{Y: -3000}) // to the bottom: the target is far above the viewport
	h.Render()
	hover = hover[:0]

	// Pick up the draggable at the bottom of the viewport and carry it to
	// where the target used to be painted; only filler is there now.
	h.DragTo(geom.Pt{X: 160, Y: 220}, geom.Pt{X: 160, Y: 50})
	h.Render()
	for _, hv := range hover {
		if hv {
			t.Fatal("a culled DropTarget reported hovering from its stale rect")
		}
	}
	h.Release(geom.Pt{X: 160, Y: 50})
	if dropped != 0 {
		t.Fatal("the payload was dropped onto a target that is not on screen")
	}
}

// A Draggable with no DragHost above it does nothing, rather than crashing.
//
// start() declined without a session, but OnDrag then moved the session
// anyway, so the first move past the slop dereferenced nil in any tree the
// app runner did not build — an embedder's, or a test's.
func TestDraggableWithoutDragHostIsInert(t *testing.T) {
	in := input.New()
	o := &widget.Owner{Input: in}
	o.SetRoot(widget.Draggable{Payload: 1, Child: widget.Sized{W: 50, H: 50}})
	box := o.RootBox()
	box.Layout(layout.Tight(geom.Size{W: 50, H: 50}))
	var g *widget.Gestures
	for _, hit := range layout.HitTest(box, geom.Pt{X: 10, Y: 10}) {
		if b, ok := hit.Box.(*widget.InteractiveBox); ok {
			g = b.GestureHandler()
		}
	}
	if g == nil {
		t.Fatal("no InteractiveBox under the pointer")
	}
	in.HandlePointer(shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: 10, Y: 10}})
	g.OnPress(geom.Pt{X: 10, Y: 10})
	in.HandlePointer(shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: 40, Y: 40}})
	g.OnDrag(geom.Pt{X: 40, Y: 40}, geom.Pt{X: 30, Y: 30}) // panicked here
	g.OnRelease()
}
