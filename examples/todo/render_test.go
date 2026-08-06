package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func newHeadless(t *testing.T) (*app.Headless, *todoState) {
	t.Helper()
	var st *todoState
	stateHook = func(s *todoState) { st = s }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(Todo{}, app.Config{
		Size:       geom.Size{W: 440, H: 560},
		Background: BG,
		Font:       goregular.TTF,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount + first layout
	if st == nil {
		t.Fatal("state hook did not fire")
	}
	return h, st
}

// rowPoint scans down the window center until the pointer hovers row i.
func rowPoint(t *testing.T, h *app.Headless, st *todoState, i int) geom.Pt {
	t.Helper()
	for y := float32(0); y < 560; y += 4 {
		p := geom.Pt{X: 220, Y: y}
		h.Move(p)
		if st.hover == i {
			return p
		}
	}
	t.Fatalf("row %d not found by hover scan", i)
	return geom.Pt{}
}

func TestTypingAddsItem(t *testing.T) {
	h, st := newHeadless(t)
	n := len(st.items)

	h.Type("ship it")
	if st.input != "ship it" {
		t.Fatalf("input = %q", st.input)
	}
	h.Key(shell.KeyEnter)
	if len(st.items) != n+1 || st.items[n].text != "ship it" {
		t.Fatalf("items = %+v", st.items)
	}
	if st.input != "" {
		t.Fatal("input should clear on Enter")
	}

	h.Type("x")
	h.Key(shell.KeyBackspace)
	if st.input != "" {
		t.Fatal("backspace should delete")
	}
}

func TestTapTogglesAndHoverTracks(t *testing.T) {
	h, st := newHeadless(t)
	p := rowPoint(t, h, st, 0)

	was := st.items[0].done
	h.Tap(p)
	if st.items[0].done == was {
		t.Fatal("tap did not toggle row 0")
	}

	// Hover off the list clears hover.
	h.Move(geom.Pt{X: 220, Y: 555})
	if st.hover != -1 {
		t.Fatalf("hover = %d, want -1", st.hover)
	}
}

func TestHoverAnimationSettles(t *testing.T) {
	h, st := newHeadless(t)
	rowPoint(t, h, st, 0)
	steps := 0
	for h.Step(0.016) {
		if steps++; steps > 60 {
			t.Fatal("hover animation did not settle within 60 frames")
		}
	}
	if steps == 0 {
		t.Fatal("hover should have started an animation")
	}
}

func TestRenderOffscreen(t *testing.T) {
	h, st := newHeadless(t)
	rowPoint(t, h, st, 1) // leave row 1 hovered: exercises hover + delete UI
	for h.Step(0.016) {   // settle the hover animation
	}

	img := h.Render()
	if img == nil {
		t.Fatal("no image")
	}
	if b := img.Bounds(); b.Dx() != 880 || b.Dy() != 1120 {
		t.Fatalf("physical size = %dx%d, want 880x1120", b.Dx(), b.Dy())
	}
	if out := os.Getenv("GOPHICS_RENDER_OUT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
