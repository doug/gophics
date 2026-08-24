package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// dnd holds the observable results of a drag.
type dnd struct {
	scroll   *widget.ScrollController
	dropped  []string
	starts   int
	ends     []bool
	hovering map[string]bool
}

// dndApp stacks a 40pt draggable source, then a target that refuses strings,
// then one that accepts them — so a drag can be walked across both.
type dndApp struct {
	r         *dnd
	longPress bool
	scrolling bool // wrap the tree in a Scroll, as the gallery page does
}

func (a dndApp) Build(widget.Ctx) widget.Widget {
	r := a.r
	source := widget.Draggable{
		Payload:          "card-1",
		LongPressToStart: a.longPress,
		OnDragStart:      func() { r.starts++ },
		OnDragEnd:        func(ok bool) { r.ends = append(r.ends, ok) },
		Child:            widget.Sized{W: 200, H: 40},
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
	col := widget.Column(source, refusing, accepting)
	if a.scrolling {
		// The real page scrolls, and a vertical scroll competes with the drag
		// for the same movement. This is what LongPressToStart exists for.
		// Padding above so the source chip sits well inside the viewport with
		// room to scroll in both directions.
		return widget.Scroll{Axis: layout.Vertical, Controller: r.scroll, Child: widget.Column(
			widget.Sized{H: scrollPadTop}, col, widget.Sized{H: 800},
		)}
	}
	return col
}

// Source occupies y 0..40, the refusing target 40..80, the accepting one
// 80..120.
// scrollPadTop is the filler above the column in the scrolling variant; add it
// to srcY/refuseY/acceptY to get window coordinates there.
const scrollPadTop = 60

const (
	srcY    = 20
	refuseY = 60
	acceptY = 100
)

func newDnD(t *testing.T) (*app.Headless, *dnd) { return newDnDWith(t, false) }

// newDnDWith builds the same tree with the long-press-to-start variant, which
// is what anything inside a scrollable uses on touch.
func newDnDWith(t *testing.T, longPress bool) (*app.Headless, *dnd) {
	return newDnDIn(t, longPress, false)
}

func newDnDIn(t *testing.T, longPress, scrolling bool) (*app.Headless, *dnd) {
	t.Helper()
	r := &dnd{hovering: map[string]bool{}, scroll: &widget.ScrollController{}}
	h, err := app.NewHeadless(dndApp{r: r, longPress: longPress, scrolling: scrolling}, app.Config{
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

// The touch path: a long press arms the drag, then the finger moves.
//
// This is the variant that was broken, and the mouse tests above could not see
// it. A long press starts the session *before* any drag commits, and the host
// used to report "your press ended" to every box that had taken the press the
// moment a drag committed — including the box that had just won it. Draggable
// reads that as a cancelled gesture and tears the session down, so the chip
// died on the first move: pressed, and then refusing to move. Arming inside
// OnDrag (the mouse path) happens after the commit, which is the only reason
// that path survived.
func TestLongPressThenDragCarriesThePayload(t *testing.T) {
	h, r := newDnDWith(t, true)

	h.Press(geom.Pt{X: 100, Y: srcY})
	for s := 0; s < 40; s++ { // past the half-second long-press threshold
		h.Step(1.0 / 60)
	}
	h.Move(geom.Pt{X: 100, Y: acceptY})
	h.Release(geom.Pt{X: 100, Y: acceptY})
	h.Render()

	if r.starts == 0 {
		t.Fatal("the long press never started a drag; the payload could not move")
	}
	if len(r.dropped) != 1 || r.dropped[0] != "card-1" {
		t.Errorf("target received %v, want one \"card-1\"", r.dropped)
	}
}

// The gallery's arrangement: long-press-to-start, inside a vertical scroll.
// The chip and the scroller both want the same downward movement, and the
// chip is the deeper candidate, so an armed long press has to beat the scroll.
func TestLongPressDragBeatsTheScrollAroundIt(t *testing.T) {
	h, r := newDnDIn(t, true, true)

	h.Press(geom.Pt{X: 100, Y: scrollPadTop + srcY})
	for s := 0; s < 40; s++ {
		h.Step(1.0 / 60)
	}
	h.Move(geom.Pt{X: 100, Y: scrollPadTop + acceptY})
	h.Release(geom.Pt{X: 100, Y: scrollPadTop + acceptY})
	h.Render()

	if r.starts == 0 {
		t.Fatal("the scroll took the gesture; the chip never started moving")
	}
	if len(r.dropped) != 1 || r.dropped[0] != "card-1" {
		t.Errorf("target received %v, want one \"card-1\"", r.dropped)
	}
}

// Touching a chip and dragging without the long press must scroll the page.
//
// LongPressToStart means "this drag is not mine until a long press says so".
// But the chip is still a drag candidate, and it is deeper than the scroller,
// so it won the gesture anyway and then declined to act on it — its OnDrag
// returns early while unarmed. The result is a dead gesture: the chip does not
// move because it was never armed, and the page does not scroll because the
// chip took the drag. On a phone that is a chip that will not move when
// pressed, and a page that will not scroll if your finger starts on one.
func TestAnUnarmedChipLetsThePageScroll(t *testing.T) {
	h, r := newDnDIn(t, true, true)

	// A plain drag starting on the chip itself, with no pause to arm the long
	// press — the finger goes down on the chip and immediately moves up.
	h.Drag(geom.Pt{X: 100, Y: scrollPadTop + srcY}, geom.Pt{X: 100, Y: 10})
	h.Render()

	if r.starts != 0 {
		t.Fatalf("an unarmed chip started %d drags; the long press never happened", r.starts)
	}
	if off := r.scroll.Offset(); off <= 0 {
		t.Errorf("the page did not scroll (offset %v): the unarmed chip swallowed the drag", off)
	}
}
