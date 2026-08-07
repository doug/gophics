package app

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type animApp struct{ hook func(*animAppState) }

func (a animApp) CreateState() widget.State { s := &animAppState{}; s.hook = a.hook; return s }

type animAppState struct {
	widget.StateBase[animApp]
	hook func(*animAppState)
	on   bool
	last paint.Color
}

func (s *animAppState) Init(widget.Ctx) { s.hook(s) }

func (s *animAppState) Build(widget.Ctx) widget.Widget {
	target := paint.RGB(0, 0, 0)
	if s.on {
		target = paint.RGB(1, 1, 1)
	}
	return widget.AnimateColor(target, 100*time.Millisecond, func(c paint.Color) widget.Widget {
		s.last = c
		return widget.Fill{Color: c}
	})
}

func TestAnimatedColorTweens(t *testing.T) {
	var st *animAppState
	h, err := NewHeadless(animApp{hook: func(s *animAppState) { st = s }},
		Config{Size: geom.Size{W: 80, H: 80}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st.last != (paint.RGB(0, 0, 0)) {
		t.Fatalf("initial should be black, got %+v", st.last)
	}
	// Flip target to white; over ~100ms it should pass through greys.
	st.on = true
	h.core.Owner.RebuildAll()
	h.Render()
	h.Step(0.03) // ~30% through
	h.Render()
	mid := st.last
	if mid.R <= 0.01 || mid.R >= 0.99 {
		t.Fatalf("mid-tween should be grey, got R=%v", mid.R)
	}
	// Settle.
	for h.Step(0.016) {
		h.Render()
	}
	if st.last.R < 0.99 {
		t.Fatalf("did not settle at white, R=%v", st.last.R)
	}
	// Pixel confirms.
	img := h.Render()
	if r, _, _, _ := img.At(40, 40).RGBA(); r>>8 < 250 {
		t.Fatalf("surface not white after settle: r=%d", r>>8)
	}
}
