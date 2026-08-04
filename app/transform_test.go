package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type scaleApp struct{ hook func(*scaleState) }

func (a scaleApp) CreateState() widget.State { s := &scaleState{}; s.hook = a.hook; return s }

type scaleState struct {
	widget.StateBase[scaleApp]
	hook   func(*scaleState)
	tapped bool
}

func (s *scaleState) Init(widget.Ctx) { s.hook(s) }

func (s *scaleState) Build(widget.Ctx) widget.Widget {
	// A 50×50 button at the top-left, scaled 2× about its origin: it visually
	// covers [0,0]–[100,100] but its layout box is still 50×50.
	btn := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.tapped = true }) }},
		Child:   widget.Sized{W: 50, H: 50},
	}
	return widget.Align{X: 0, Y: 0, Child: widget.Scale(2, btn)}
}

func scaleHarness(t *testing.T) (*Headless, *scaleState) {
	t.Helper()
	var st *scaleState
	h, err := NewHeadless(scaleApp{hook: func(s *scaleState) { st = s }},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestTransformHitTestMapsThroughScale(t *testing.T) {
	h, st := scaleHarness(t)

	// (80,80) is outside the 50×50 layout box but inside the 2× visual — the
	// inverse maps it to (40,40), which is on the button.
	h.Tap(geom.Pt{X: 80, Y: 80})
	if !st.tapped {
		t.Fatal("tap inside the scaled button did not register")
	}
}

func TestTransformHitTestRejectsOutside(t *testing.T) {
	h, st := scaleHarness(t)

	// (150,150) → inverse (75,75), past the 50×50 button: no hit.
	h.Tap(geom.Pt{X: 150, Y: 150})
	if st.tapped {
		t.Fatal("tap outside the scaled button should not register")
	}
}
