package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/widget"
)

type refreshApp struct{ hook func(*refreshState) }

func (a refreshApp) CreateState() widget.State { s := &refreshState{}; s.hook = a.hook; return s }

type refreshState struct {
	widget.StateBase[refreshApp]
	hook         func(*refreshState)
	refreshCount int
	busy         bool
}

func (s *refreshState) Init(widget.Ctx) { s.hook(s) }

func (s *refreshState) Build(widget.Ctx) widget.Widget {
	return widget.Scroll{
		Refreshing: s.busy,
		OnRefresh:  func() { s.SetState(func() { s.refreshCount++; s.busy = true }) },
		Child:      widget.Sized{W: 200, H: 600},
	}
}

func refreshHarness(t *testing.T) (*Headless, *refreshState) {
	t.Helper()
	var st *refreshState
	h, err := NewHeadless(refreshApp{hook: func(s *refreshState) { st = s }},
		Config{Size: geom.Size{W: 220, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

// pull drags down from the top by dy logical px (one gesture) and releases.
func pull(h *Headless, dy float32) {
	h.Move(geom.Pt{X: 100, Y: 30})
	h.DragTo(geom.Pt{X: 100, Y: 30}, geom.Pt{X: 100, Y: 30 + dy})
	h.Release(geom.Pt{X: 100, Y: 30 + dy})
	h.Render()
}

func TestPullToRefreshTriggersPastThreshold(t *testing.T) {
	h, st := refreshHarness(t)

	// A short pull (overscroll ~30 < trigger 64) must not refresh.
	pull(h, 60)
	if st.refreshCount != 0 {
		t.Fatalf("short pull fired refresh: count=%d", st.refreshCount)
	}
	if st.busy {
		t.Fatalf("short pull set busy")
	}

	// A long pull past the trigger fires exactly once.
	pull(h, 200)
	if st.refreshCount != 1 {
		t.Fatalf("long pull refresh count=%d, want 1", st.refreshCount)
	}
	if !st.busy {
		t.Fatalf("long pull did not set busy")
	}

	// While busy, another long pull must not re-fire (latched).
	pull(h, 200)
	if st.refreshCount != 1 {
		t.Fatalf("refresh re-fired while busy: count=%d", st.refreshCount)
	}
}

func TestPullToRefreshRetractsAndRearms(t *testing.T) {
	h, st := refreshHarness(t)

	pull(h, 200)
	if st.refreshCount != 1 || !st.busy {
		t.Fatalf("first refresh not triggered: count=%d busy=%v", st.refreshCount, st.busy)
	}

	// App finishes work: clear busy. The indicator retracts over a few frames
	// (must not panic), and the machine re-arms.
	st.SetState(func() { st.busy = false })
	for i := 0; i < 40; i++ {
		h.Step(0.016)
		h.Render()
	}

	// A fresh long pull fires again now that we've re-armed.
	pull(h, 200)
	if st.refreshCount != 2 {
		t.Fatalf("did not re-arm after retract: count=%d", st.refreshCount)
	}
}
