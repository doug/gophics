package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

type lpApp struct{ hook func(*lpState) }

func (a lpApp) CreateState() widget.State { s := &lpState{}; s.hook = a.hook; return s }

type lpState struct {
	widget.StateBase[lpApp]
	hook  func(*lpState)
	taps  int
	longs int
}

func (s *lpState) Init(widget.Ctx) { s.hook(s) }
func (s *lpState) Build(widget.Ctx) widget.Widget {
	return widget.Center(widget.Interactive{
		Handler: widget.Handler{
			OnTap:       func() { s.taps++ },
			OnLongPress: func() { s.longs++ },
		},
		Child: widget.Sized{W: 100, H: 100},
	})
}

func lpHarness(t *testing.T) (*Headless, *lpState) {
	t.Helper()
	var st *lpState
	h, err := NewHeadless(lpApp{hook: func(s *lpState) { st = s }},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestLongPressFires(t *testing.T) {
	h, st := lpHarness(t)
	// Press and hold, no release, advance time past the threshold.
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 100, Y: 100}})
	for i := 0; i < 40 && st.longs == 0; i++ {
		h.Step(0.02) // 20ms/frame; ~0.5s → fires
	}
	if st.longs != 1 {
		t.Fatalf("long-press fired %d times, want 1", st.longs)
	}
	// Release after long-press: no tap.
	h.core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: 100, Y: 100}})
	if st.taps != 0 {
		t.Fatalf("long-press consumed the gesture; taps should be 0, got %d", st.taps)
	}
}

func TestQuickTapNotLongPress(t *testing.T) {
	h, st := lpHarness(t)
	h.Tap(geom.Pt{X: 100, Y: 100}) // down+up immediately
	if st.taps != 1 || st.longs != 0 {
		t.Fatalf("quick tap: taps=%d longs=%d", st.taps, st.longs)
	}
}

func TestMoveCancelsLongPress(t *testing.T) {
	h, st := lpHarness(t)
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 100, Y: 100}})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: geom.Pt{X: 130, Y: 100}}) // past slop
	for i := 0; i < 40; i++ {
		h.Step(0.02)
	}
	if st.longs != 0 {
		t.Fatalf("moving should cancel long-press, got %d", st.longs)
	}
}
