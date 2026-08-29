package app

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type ascaleApp struct{ hook func(*ascaleState) }

func (a ascaleApp) CreateState() widget.State { s := &ascaleState{}; s.hook = a.hook; return s }

type ascaleState struct {
	widget.StateBase[ascaleApp]
	hook   func(*ascaleState)
	scale  float32
	tapped bool
}

func (s *ascaleState) Init(widget.Ctx) { s.hook(s); s.scale = 1 }

func (s *ascaleState) Build(widget.Ctx) widget.Widget {
	btn := widget.Interactive{
		Gestures: widget.Gestures{OnTap: func() { s.SetState(func() { s.tapped = true }) }},
		Child:    widget.Sized{W: 40, H: 40},
	}
	// A 40×40 button centered in 200×200, scaled about its center.
	return widget.Center(widget.AnimatedScale(s.scale, 80*time.Millisecond, btn))
}

func ascaleHarness(t *testing.T) (*Headless, *ascaleState) {
	t.Helper()
	var st *ascaleState
	h, err := NewHeadless(ascaleApp{hook: func(s *ascaleState) { st = s }},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestAnimatedScaleGrowsHitRegion(t *testing.T) {
	h, st := ascaleHarness(t)

	// At rest (scale 1) the button is [80,80]–[120,120]; (135,135) misses.
	h.Tap(geom.Pt{X: 135, Y: 135})
	if st.tapped {
		t.Fatal("tap outside the unscaled button should miss")
	}

	// Grow to 2×; the animation must take several frames to settle.
	st.SetState(func() { st.scale = 2 })
	h.Render() // rebuild starts the tween
	frames := 0
	for h.Step(0.016) {
		h.Render()
		if frames++; frames > 60 {
			t.Fatal("scale animation did not settle")
		}
	}
	if frames < 2 {
		t.Fatalf("scale should animate over frames, settled in %d", frames)
	}

	// Now the 80×80 scaled region covers (135,135) → inverse lands on the button.
	h.Tap(geom.Pt{X: 135, Y: 135})
	if !st.tapped {
		t.Fatal("tap inside the 2× scaled button should hit")
	}
}
