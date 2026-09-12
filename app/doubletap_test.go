package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type dtApp struct{ hook func(*dtState) }

func (a dtApp) CreateState() widget.State { s := &dtState{}; s.hook = a.hook; return s }

type dtState struct {
	widget.StateBase[dtApp]
	hook    func(*dtState)
	taps    int
	doubles int
	triples int
}

func (s *dtState) Init(widget.Ctx) { s.hook(s) }
func (s *dtState) Build(widget.Ctx) widget.Widget {
	return widget.Center(widget.Interactive{
		Gestures: widget.Gestures{
			OnTap:       func() { s.taps++ },
			OnDoubleTap: func() { s.doubles++ },
			OnTripleTap: func() { s.triples++ },
		},
		Child: widget.Sized{W: 100, H: 100},
	})
}
func dtHarness(t *testing.T) (*Headless, *dtState) {
	t.Helper()
	var st *dtState
	h, err := NewHeadless(dtApp{hook: func(s *dtState) { st = s }},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestDoubleTapFires(t *testing.T) {
	h, st := dtHarness(t)
	// Two quick taps within the window.
	h.Tap(geom.Pt{X: 100, Y: 100})
	h.Step(0.05) // still inside window
	h.Tap(geom.Pt{X: 100, Y: 100})
	if st.doubles != 1 {
		t.Fatalf("doubles=%d, want 1", st.doubles)
	}
	if st.taps != 0 {
		t.Fatalf("double-tap should cancel the deferred single; taps=%d", st.taps)
	}
}

func TestSingleTapDefersThenFires(t *testing.T) {
	h, st := dtHarness(t)
	h.Tap(geom.Pt{X: 100, Y: 100})
	if st.taps != 0 {
		t.Fatalf("single tap should defer when OnDoubleTap set; taps=%d", st.taps)
	}
	// Let the window expire.
	for i := 0; i < 30 && st.taps == 0; i++ {
		h.Step(0.02)
	}
	if st.taps != 1 || st.doubles != 0 {
		t.Fatalf("deferred single should fire once: taps=%d doubles=%d", st.taps, st.doubles)
	}
}

// A double tap opens the window for a third. That window is frame time, and an
// idle app renders no frames — so the core must ask for them while it is open,
// or a click a minute later would still count as the third.
func TestTripleTapWindowClosesWhileIdle(t *testing.T) {
	h, st := dtHarness(t)
	p := geom.Pt{X: 100, Y: 100}
	h.Tap(p)
	h.Tap(p)
	if st.doubles != 1 {
		t.Fatalf("doubles = %d, want 1", st.doubles)
	}
	if !h.core.longPressPending() {
		t.Fatal("after a double tap the core must keep frames coming for the triple window")
	}
	for i := 0; i < 40 && h.core.longPressPending(); i++ {
		h.Step(0.016)
	}
	if h.core.longPressPending() {
		t.Fatal("the triple window never closed")
	}
	h.Tap(p)
	if st.triples != 0 {
		t.Fatalf("a tap after the window counted as a triple (triples=%d)", st.triples)
	}
}
