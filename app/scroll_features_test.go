package app

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/widget"
)

type feedApp struct {
	hook func(*feedAppState)
}

func (a feedApp) CreateState() widget.State { s := &feedAppState2{}; s.hook = a.hook; return s }

// reuse a distinct state type name to avoid clashes
type feedAppState = feedAppState2

type feedAppState2 struct {
	widget.StateBase[feedApp]
	hook    func(*feedAppState2)
	count   int
	loads   int
	ctrl    widget.ScrollController
}

func (s *feedAppState2) Init(widget.Ctx) { s.hook(s); s.count = 20 }

func (s *feedAppState2) Build(widget.Ctx) widget.Widget {
	return widget.LazyList{
		Count:           s.count,
		EstimatedExtent: 40,
		Controller:      &s.ctrl,
		OnEndReached: func() {
			s.SetState(func() { s.loads++; s.count += 20 })
		},
		Build: func(i int) widget.Widget { return widget.Sized{W: 200, H: 40} },
	}
}

func scrollHarness(t *testing.T) (*Headless, *feedAppState2) {
	t.Helper()
	var st *feedAppState2
	h, err := NewHeadless(feedApp{hook: func(s *feedAppState2) { st = s }},
		Config{Size: geom.Size{W: 220, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestInfiniteScrollOnEndReached(t *testing.T) {
	h, st := scrollHarness(t)
	h.Move(geom.Pt{X: 100, Y: 150})
	// Scroll toward the bottom; nearing the end grows the feed.
	for i := 0; i < 30 && st.loads < 2; i++ {
		h.Scroll(geom.Pt{Y: -300})
		h.Render()
	}
	if st.loads < 1 {
		t.Fatalf("OnEndReached never fired (loads=%d count=%d)", st.loads, st.count)
	}
	if st.count < 40 {
		t.Fatalf("feed did not grow: count=%d", st.count)
	}
}

func TestScrollControllerJumpAndAnimate(t *testing.T) {
	h, st := scrollHarness(t)
	st.ctrl.JumpTo(500)
	h.Render()
	if st.ctrl.Offset() < 400 {
		t.Fatalf("JumpTo offset = %v, want ~500 (clamped to max)", st.ctrl.Offset())
	}
	// Animate back to top.
	st.ctrl.AnimateTo(0, 100*time.Millisecond)
	settled := false
	for i := 0; i < 60; i++ {
		h.Step(0.016)
		h.Render()
		if st.ctrl.Offset() < 1 {
			settled = true
			break
		}
	}
	if !settled {
		t.Fatalf("AnimateTo did not reach 0, offset=%v", st.ctrl.Offset())
	}
}
