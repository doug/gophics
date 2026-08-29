package app

import (
	"slices"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type dmApp struct{ hook func(*dmState) }

func (a dmApp) CreateState() widget.State { s := &dmState{}; s.hook = a.hook; return s }

type dmState struct {
	widget.StateBase[dmApp]
	hook  func(*dmState)
	items []int
}

func (s *dmState) Init(widget.Ctx) { s.hook(s); s.items = []int{0, 1, 2} }

func (s *dmState) remove(id int) {
	s.SetState(func() {
		if i := slices.Index(s.items, id); i >= 0 {
			s.items = slices.Delete(s.items, i, i+1)
		}
	})
}

func (s *dmState) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 0, len(s.items))
	for _, id := range s.items {
		rows = append(rows, widget.WithKey{Key: id, Child: widget.Dismissible{
			OnDismissed: func() { s.remove(id) },
			Child:       widget.Sized{W: 200, H: 40},
		}})
	}
	col := widget.Column(rows...)
	col.CrossAlign = 0 // start
	return col
}

func dmHarness(t *testing.T) (*Headless, *dmState) {
	t.Helper()
	var st *dmState
	h, err := NewHeadless(dmApp{hook: func(s *dmState) { st = s }},
		Config{Size: geom.Size{W: 220, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

// swipe drags horizontally by dx at row y and releases, then settles.
func swipe(h *Headless, y, dx float32) {
	h.DragTo(geom.Pt{X: 40, Y: y}, geom.Pt{X: 40 + dx, Y: y})
	h.Release(geom.Pt{X: 40 + dx, Y: y})
	for range 40 {
		h.Step(0.016)
		h.Render()
	}
}

func TestDismissibleSwipeRemoves(t *testing.T) {
	h, st := dmHarness(t)
	if len(st.items) != 3 {
		t.Fatalf("setup: items=%v", st.items)
	}
	// Swipe the middle row (y in [40,80)) far right, past the threshold.
	swipe(h, 60, 150)
	if len(st.items) != 2 || slices.Contains(st.items, 1) {
		t.Fatalf("middle row not dismissed: items=%v", st.items)
	}
}

func TestDismissibleShortSwipeSpringsBack(t *testing.T) {
	h, st := dmHarness(t)
	// A small swipe (well under 0.4*200=80px) must not dismiss.
	swipe(h, 20, 30)
	if len(st.items) != 3 {
		t.Fatalf("short swipe dismissed a row: items=%v", st.items)
	}
}

func TestDismissibleDirectionConstraint(t *testing.T) {
	// Verify a DismissLeft row ignores a rightward swipe.
	var st *dirState
	h, err := NewHeadless(dirApp{hook: func(s *dirState) { st = s }},
		Config{Size: geom.Size{W: 220, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	// Rightward swipe on a left-only row: no dismissal.
	swipe(h, 20, 150)
	if len(st.items) != 3 {
		t.Fatalf("rightward swipe dismissed a left-only row: items=%v", st.items)
	}
	// Leftward swipe past threshold dismisses.
	swipe(h, 20, -150)
	if len(st.items) != 2 {
		t.Fatalf("leftward swipe did not dismiss: items=%v", st.items)
	}
}

type dirApp struct{ hook func(*dirState) }

func (a dirApp) CreateState() widget.State { s := &dirState{}; s.hook = a.hook; return s }

type dirState struct {
	widget.StateBase[dirApp]
	hook  func(*dirState)
	items []int
}

func (s *dirState) Init(widget.Ctx) { s.hook(s); s.items = []int{0, 1, 2} }

func (s *dirState) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 0, len(s.items))
	for _, id := range s.items {
		rows = append(rows, widget.WithKey{Key: id, Child: widget.Dismissible{
			Direction: widget.DismissLeft,
			OnDismissed: func() {
				s.SetState(func() {
					if i := slices.Index(s.items, id); i >= 0 {
						s.items = slices.Delete(s.items, i, i+1)
					}
				})
			},
			Child: widget.Sized{W: 200, H: 40},
		}})
	}
	return widget.Column(rows...)
}
