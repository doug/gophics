package app

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type aliasApp struct{ hook func(*aliasState) }

func (a aliasApp) CreateState() widget.State { s := &aliasState{}; s.hook = a.hook; return s }

type aliasState struct {
	widget.StateBase[aliasApp]
	hook  func(*aliasState)
	color paint.Color
}

func (s *aliasState) Init(widget.Ctx) { s.hook(s) }

func (s *aliasState) Build(widget.Ctx) widget.Widget {
	return widget.Fill{Color: s.color}
}

// TestSurfaceBufAliasing_Repro reproduces the aliasing hazard flagged by
// critiques 01a035b7-ecda/01a035b7-cd17 on solution 01a035af: that solution's
// diff (paint.go's Painter.surfaceBufs [2]*image.RGBA ping-pong) is already
// merged onto main (commit b28be46) despite both critiques being open, so
// this repro runs directly against the current tree.
//
// A caller that holds an image.Image returned by Painter.Image() across two
// further *changed* renders gets that image silently mutated in place,
// because the 2-slot ping-pong reuses the same backing array every other
// frame.
func TestSurfaceBufAliasing_Repro(t *testing.T) {
	var st *aliasState
	h, err := NewHeadless(aliasApp{hook: func(s *aliasState) { st = s }}, Config{
		Size: geom.Size{W: 20, H: 20}, Background: paint.RGB(0, 0, 0), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.color = paint.RGB(1, 0, 0) // red
	h.core.Owner.RebuildAll()
	img0 := h.Render()
	rgba0, ok := img0.(*image.RGBA)
	if !ok {
		t.Fatalf("expected *image.RGBA, got %T", img0)
	}
	r0, g0, b0, _ := rgba0.At(10, 10).RGBA()
	t.Logf("frame0 (retained): r=%d g=%d b=%d", r0>>8, g0>>8, b0>>8)

	st.color = paint.RGB(0, 1, 0) // green
	h.core.Owner.RebuildAll()
	h.Render()

	st.color = paint.RGB(0, 0, 1) // blue
	h.core.Owner.RebuildAll()
	h.Render()

	r1, g1, b1, _ := rgba0.At(10, 10).RGBA()
	t.Logf("frame0 (retained, after 2 more renders): r=%d g=%d b=%d", r1>>8, g1>>8, b1>>8)
	if r0 != r1 || g0 != g1 || b0 != b1 {
		t.Fatalf("retained image mutated in place: was r=%d g=%d b=%d, now r=%d g=%d b=%d (surfaceBufs aliasing)",
			r0>>8, g0>>8, b0>>8, r1>>8, g1>>8, b1>>8)
	}
}
