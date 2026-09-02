package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// --- widget fix: keyboard focus is released when the focused widget unmounts ---

var focusToggle *focusState

type focusApp struct{}

func (focusApp) CreateState() widget.State { return &focusState{show: true} }

type focusState struct {
	widget.StateBase[focusApp]
	show bool
}

func (s *focusState) Init(widget.Ctx) { focusToggle = s }

func (s *focusState) Build(widget.Ctx) widget.Widget {
	if s.show {
		return widget.Interactive{Gestures: widget.Gestures{OnKey: func(shell.Key) {}}, Child: widget.Sized{W: 10, H: 10}}
	}
	return widget.Sized{W: 10, H: 10}
}

func TestKeyboardFocusClearedOnUnmount(t *testing.T) {
	focusToggle = nil
	h, err := NewHeadless(focusApp{}, Config{Size: geom.Size{W: 100, H: 100}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mounts the focusable → autofocus takes it
	if h.core.Owner.KeyboardTarget == nil {
		t.Fatal("focusable Interactive did not autofocus on mount")
	}

	focusToggle.SetState(func() { focusToggle.show = false }) // remove it from the tree
	h.Render()
	if h.core.Owner.KeyboardTarget != nil {
		t.Fatal("KeyboardTarget was not cleared after the focused widget unmounted")
	}

	focusToggle.SetState(func() { focusToggle.show = true }) // a new focusable should autofocus again
	h.Render()
	if h.core.Owner.KeyboardTarget == nil {
		t.Fatal("a new focusable did not autofocus after focus was released")
	}
}

// --- widget fix: Flex tolerates a Flexible wrapping a nil child ---

type nilFlexApp struct{}

func (nilFlexApp) Build(widget.Ctx) widget.Widget {
	return widget.Row(widget.Expand(nil), widget.Text{Value: "x", Size: 12})
}

func TestFlexFlexibleNilChild(t *testing.T) {
	h, err := NewHeadless(nilFlexApp{}, Config{Size: geom.Size{W: 100, H: 40}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount + attach — must not panic on Expand(nil)
	h.Render() // reconcile again
}

// --- app fix: a deferred single-tap isn't lost when a new tap lands elsewhere ---

type tapCounts struct{ tap, dbl int }

type twoTapApp struct{ a, b *tapCounts }

func (t twoTapApp) Build(widget.Ctx) widget.Widget {
	mk := func(c *tapCounts) widget.Widget {
		return widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { c.tap++ }, OnDoubleTap: func() { c.dbl++ }},
			Child:    widget.Sized{W: 40, H: 40},
		}
	}
	return widget.Column(mk(t.a), mk(t.b))
}

func TestPendingTapFlushedOnNewTap(t *testing.T) {
	a, b := &tapCounts{}, &tapCounts{}
	h, err := NewHeadless(twoTapApp{a, b}, Config{Size: geom.Size{W: 80, H: 80}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // layout so hit geometry exists

	h.Tap(geom.Pt{X: 20, Y: 20}) // tap A → deferred (A has OnDoubleTap)
	h.Tap(geom.Pt{X: 20, Y: 60}) // tap B → must flush A's OnTap, defer B
	if a.tap != 1 {
		t.Fatalf("A.OnTap dropped when B was tapped: got %d, want 1", a.tap)
	}
	if b.tap != 0 {
		t.Fatalf("B.OnTap fired before its double-tap window: got %d", b.tap)
	}
	h.Step(doubleTapWindow + 0.1) // window expires → B's deferred tap fires
	if b.tap != 1 {
		t.Fatalf("B.OnTap not fired after the window: got %d, want 1", b.tap)
	}
	if a.dbl != 0 || b.dbl != 0 {
		t.Fatalf("spurious double taps: a=%d b=%d", a.dbl, b.dbl)
	}
}
