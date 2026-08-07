package app

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type opApp struct{ alpha float32 }

func (a opApp) Build(widget.Ctx) widget.Widget {
	return widget.Fill{Color: paint.RGB(0, 0, 0), Child: widget.Center(
		widget.Opacity{Alpha: a.alpha, Child: widget.Sized{W: 40, H: 40, Child: widget.Fill{Color: paint.RGB(1, 1, 1)}}},
	)}
}

func TestGroupOpacityBlends(t *testing.T) {
	sample := func(alpha float32) uint32 {
		h, err := NewHeadless(opApp{alpha: alpha}, Config{
			Size: geom.Size{W: 80, H: 80}, Background: paint.RGB(0, 0, 0), Font: goregular.TTF}, 1)
		if err != nil {
			t.Fatal(err)
		}
		img := h.Render()
		r, _, _, _ := img.At(40, 40).RGBA()
		return r >> 8
	}
	if v := sample(1); v < 250 {
		t.Fatalf("alpha 1 should be white, got %d", v)
	}
	if v := sample(0); v > 5 {
		t.Fatalf("alpha 0 should be black, got %d", v)
	}
	if v := sample(0.5); v < 90 || v > 165 {
		t.Fatalf("alpha 0.5 should be mid-grey, got %d", v)
	}
}

type fadeApp struct{ hook func(*fadeState) }

func (a fadeApp) CreateState() widget.State { s := &fadeState{}; s.hook = a.hook; return s }

type fadeState struct {
	widget.StateBase[fadeApp]
	hook func(*fadeState)
	show bool
}

func (s *fadeState) Init(widget.Ctx) { s.hook(s) }

func (s *fadeState) Build(widget.Ctx) widget.Widget {
	target := float32(0)
	if s.show {
		target = 1
	}
	return widget.Fill{Color: paint.RGB(0, 0, 0), Child: widget.Center(
		widget.AnimateFloat(target, 100*time.Millisecond, func(a float32) widget.Widget {
			return widget.Opacity{Alpha: a, Child: widget.Sized{W: 40, H: 40, Child: widget.Fill{Color: paint.RGB(1, 1, 1)}}}
		}),
	)}
}

func TestAnimatedOpacityFadesIn(t *testing.T) {
	var st *fadeState
	h, err := NewHeadless(fadeApp{hook: func(s *fadeState) { st = s }}, Config{
		Size: geom.Size{W: 80, H: 80}, Background: paint.RGB(0, 0, 0), Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if r, _, _, _ := h.Render().At(40, 40).RGBA(); r>>8 > 5 {
		t.Fatalf("starts hidden, got %d", r>>8)
	}
	st.show = true
	h.core.Owner.RebuildAll()
	h.Render()
	h.Step(0.04)
	h.Render()
	if r, _, _, _ := h.Render().At(40, 40).RGBA(); r>>8 < 20 || r>>8 > 235 {
		t.Fatalf("mid-fade should be partial grey, got %d", r>>8)
	}
	for h.Step(0.016) {
		h.Render()
	}
	if r, _, _, _ := h.Render().At(40, 40).RGBA(); r>>8 < 250 {
		t.Fatalf("faded in should be white, got %d", r>>8)
	}
}
