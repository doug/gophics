package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type chatApp struct{ hook func(*chatState) }

func (a chatApp) CreateState() widget.State { s := &chatState{}; s.hook = a.hook; return s }

type chatState struct {
	widget.StateBase[chatApp]
	hook   func(*chatState)
	n      int // message count
	tapped int // index of the last tapped row
	ctrl   widget.ScrollController
}

func (s *chatState) Init(widget.Ctx) { s.hook(s); s.n = 20; s.tapped = -1 }

func (s *chatState) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 0, s.n)
	for i := 0; i < s.n; i++ {
		i := i
		rows = append(rows, widget.WithKey{Key: i, Child: widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.tapped = i }) }},
			Child:   widget.Sized{W: 200, H: 40},
		}})
	}
	col := widget.Column(rows...)
	col.CrossAlign = 0
	return widget.Scroll{Reverse: true, Controller: &s.ctrl, Child: col}
}

func chatHarness(t *testing.T) (*Headless, *chatState) {
	t.Helper()
	var st *chatState
	h, err := NewHeadless(chatApp{hook: func(s *chatState) { st = s }},
		Config{Size: geom.Size{W: 220, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestReverseListAnchorsToBottom(t *testing.T) {
	h, st := chatHarness(t)
	h.Render()

	// A reverse list rests at the end: offset 0, newest (row 19) at bottom.
	if off := st.ctrl.Offset(); off != 0 {
		t.Fatalf("reverse list rest offset = %v, want 0", off)
	}
	// Tapping the bottom row hits the last message.
	h.Tap(geom.Pt{X: 100, Y: 190})
	if st.tapped != st.n-1 {
		t.Fatalf("bottom row tapped = %d, want %d (newest)", st.tapped, st.n-1)
	}
}

func TestReverseListStaysPinnedOnAppend(t *testing.T) {
	h, st := chatHarness(t)
	h.Render()

	// Append messages; a bottom-anchored list stays pinned to the newest.
	st.SetState(func() { st.n += 5 })
	h.Render()
	if off := st.ctrl.Offset(); off != 0 {
		t.Fatalf("offset after append = %v, want 0 (pinned to end)", off)
	}
	h.Tap(geom.Pt{X: 100, Y: 190})
	if st.tapped != st.n-1 {
		t.Fatalf("after append, bottom row = %d, want %d", st.tapped, st.n-1)
	}
}

// lazyChatApp exercises the reverse path through LazyList (virtualized).
type lazyChatApp struct{ hook func(*lazyChatState) }

func (a lazyChatApp) CreateState() widget.State {
	s := &lazyChatState{}
	s.hook = a.hook
	return s
}

type lazyChatState struct {
	widget.StateBase[lazyChatApp]
	hook   func(*lazyChatState)
	n      int
	tapped int
}

func (s *lazyChatState) Init(widget.Ctx) { s.hook(s); s.n = 500; s.tapped = -1 }

func (s *lazyChatState) Build(widget.Ctx) widget.Widget {
	return widget.LazyList{
		Count:           s.n,
		EstimatedExtent: 40,
		Reverse:         true,
		Build: func(i int) widget.Widget {
			return widget.Interactive{
				Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.tapped = i }) }},
				Child:   widget.Sized{W: 200, H: 40},
			}
		},
	}
}

func TestReverseLazyListAnchorsToBottom(t *testing.T) {
	var st *lazyChatState
	h, err := NewHeadless(lazyChatApp{hook: func(s *lazyChatState) { st = s }},
		Config{Size: geom.Size{W: 220, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Render() // let the offset/extent settle

	// The newest message (index 499) must be windowed and sit at the bottom.
	h.Tap(geom.Pt{X: 100, Y: 190})
	if st.tapped != st.n-1 {
		t.Fatalf("reverse LazyList bottom row = %d, want %d (newest)", st.tapped, st.n-1)
	}
}

func TestReverseListScrollUpRevealsOlder(t *testing.T) {
	h, st := chatHarness(t)
	h.Render()

	// Scroll up (wheel up = positive Y here reveals older content). Drag the
	// content down to move toward the top/oldest.
	h.Move(geom.Pt{X: 100, Y: 100})
	h.Drag(geom.Pt{X: 100, Y: 40}, geom.Pt{X: 100, Y: 200}) // finger down → older
	h.Render()
	if off := st.ctrl.Offset(); off <= 0 {
		t.Fatalf("scrolling up did not move offset off the end: %v", off)
	}
	// Now the bottom row is no longer the newest.
	h.Tap(geom.Pt{X: 100, Y: 190})
	if st.tapped == st.n-1 {
		t.Fatalf("after scrolling up, bottom row is still the newest (%d)", st.tapped)
	}
}
