package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// touchField mounts an Obscure field under a bare Owner whose pointer is a
// finger, and returns its state and gesture handler.
func touchField(t *testing.T, o *Owner) (*textFieldState, *Gestures) {
	t.Helper()
	in := input.New()
	in.HandlePointer(shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: 5, Y: 5}, Source: shell.SourceTouch})
	o.Input = in
	o.Painter = paint.NewPainter()
	o.SetRoot(Sized{W: 200, Child: TextField{Obscure: true}})
	box := o.RootBox()
	box.Layout(layout.Tight(geom.Size{W: 200, H: 40}))
	var g *Gestures
	for _, hit := range layout.HitTest(box, geom.Pt{X: 5, Y: 5}) {
		if b, ok := hit.Box.(*InteractiveBox); ok {
			g = b.GestureHandler()
		}
	}
	if g == nil {
		t.Fatal("no field under the pointer")
	}
	return digState[TextField](o.root).(*textFieldState), g
}

// A password field typed into by a finger shows the last character for a
// moment, then masks it from a timer. The timer's only way back to the UI
// goroutine is Owner.Post; with no runner attached there is none, and the
// callback dereferenced it on the timer goroutine, where nothing recovers.
//
// startBlink already declines to arm without a Post; revealLast now does the
// same, and the glimpse lasts until the next keystroke masks it.
func TestRevealTimerNotArmedWithoutPost(t *testing.T) {
	s, g := touchField(t, &Owner{})
	g.OnText("a")
	if !s.revealOn {
		t.Fatal("a finger's keystroke was not revealed")
	}
	if s.revealTimer != nil {
		t.Fatal("reveal timer armed with no Post to fire through; it would crash the process when it fired")
	}
}

// With a runner the timer is armed, and unmounting the field stops it: a
// field that leaves the tree mid-glimpse must not wake the UI for a state
// nobody can see.
func TestRevealTimerStopsOnDispose(t *testing.T) {
	o := &Owner{Post: func(fn func()) {}}
	s, g := touchField(t, o)
	g.OnText("a")
	if s.revealTimer == nil {
		t.Fatal("reveal timer not armed although a Post hook exists")
	}
	o.SetRoot(Text{Value: "gone"})
	o.FlushBuilds()
	if s.revealTimer != nil {
		t.Fatal("reveal timer still armed after the field unmounted")
	}
}
