package app

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/theme"
	"github.com/doug/gossamer/widget"
)

type controlsApp struct{ hook func(*controlsState) }

func (a controlsApp) CreateState() widget.State { s := &controlsState{}; s.hook = a.hook; return s }

type controlsState struct {
	widget.StateBase[controlsApp]
	hook   func(*controlsState)
	sw, cb bool
	slider float32
	radio  int
}

func (s *controlsState) Init(widget.Ctx) { s.hook(s); s.slider = 0.3 }

func (s *controlsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Dark()
	col := widget.Column(
		theme.Switch{On: s.sw, OnChange: func(v bool) { s.SetState(func() { s.sw = v }) }},
		widget.Sized{H: 16},
		theme.Checkbox{Checked: s.cb, Label: "Enable", OnChange: func(v bool) { s.SetState(func() { s.cb = v }) }},
		widget.Sized{H: 16},
		theme.Slider{Value: s.slider, OnChange: func(v float32) { s.SetState(func() { s.slider = v }) }},
		widget.Sized{H: 16},
		theme.Radio{Selected: s.radio == 0, Label: "One", OnSelect: func() { s.SetState(func() { s.radio = 0 }) }},
		theme.Radio{Selected: s.radio == 1, Label: "Two", OnSelect: func() { s.SetState(func() { s.radio = 1 }) }},
	)
	col.CrossAlign = 0 // CrossStart
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 24, Child: col}}}
}

func controlsHarness(t *testing.T) (*Headless, *controlsState) {
	t.Helper()
	var st *controlsState
	h, err := NewHeadless(controlsApp{hook: func(s *controlsState) { st = s }}, Config{
		Size: geom.Size{W: 320, H: 320}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestSwitchToggles(t *testing.T) {
	h, st := controlsHarness(t)
	h.Tap(geom.Pt{X: 44, Y: 37}) // switch is at top-left inside padding(24)
	if !st.sw {
		t.Fatal("switch did not turn on")
	}
	h.Tap(geom.Pt{X: 44, Y: 37})
	if st.sw {
		t.Fatal("switch did not turn off")
	}
}

func TestSliderDragSetsValue(t *testing.T) {
	h, st := controlsHarness(t)
	h.Render() // establish slider width via paint
	// Slider sits below switch+checkbox; drag near its right end.
	// Find it by scanning: press at increasing y until value changes.
	start := st.slider
	for y := float32(60); y < 260; y += 4 {
		h.DragTo(geom.Pt{X: 40, Y: y}, geom.Pt{X: 280, Y: y})
		h.Release(geom.Pt{X: 280, Y: y})
		if st.slider != start {
			break
		}
	}
	if st.slider <= start {
		t.Fatalf("slider drag right did not raise value: %v", st.slider)
	}
	if st.slider < 0.8 {
		t.Fatalf("drag to right end should approach 1, got %v", st.slider)
	}
}

func TestRadioSelects(t *testing.T) {
	h, st := controlsHarness(t)
	// Scan for the second radio row and tap it.
	for y := float32(120); y < 300; y += 3 {
		h.Tap(geom.Pt{X: 34, Y: y})
		if st.radio == 1 {
			break
		}
	}
	if st.radio != 1 {
		t.Fatalf("second radio not selected: %d", st.radio)
	}
}

func TestControlsGolden(t *testing.T) {
	h, st := controlsHarness(t)
	st.sw, st.cb, st.slider, st.radio = true, true, 0.6, 1
	h.Core.Owner.RebuildAll()
	h.Render() // build runs → switch starts its on-animation
	for h.Step(0.016) {
		h.Render()
	}
	img := h.Render()
	if out := os.Getenv("GOSSAMER_RENDER_OUT"); out != "" {
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}
}
