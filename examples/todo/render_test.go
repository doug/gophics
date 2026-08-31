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

// Double-clicking a todo's text opens an inline editor, and Enter commits it.
// Without this there is no way to fix a typo in an item: the row toggles, the
// swipe deletes, and the text is otherwise read-only.
func TestDoubleClickEditsItemText(t *testing.T) {
	h, st := newHeadless(t)
	p := rowPoint(t, h, st, 0)

	// The editor opens from the label, not the whole row, so find the x that
	// covers the text. Probing by st.editing rather than by typing keeps a
	// miss harmless — a stray Enter into the top field would add a todo.
	opened := openEditor(h, st, p)
	if !opened {
		t.Fatal("no x across the row opened the editor — double-click to edit is not wired")
	}

	// The editor opens with the existing text and the caret at its end, so
	// typing appends — the point is that the edit lands, not that it replaces.
	want := st.items[0].text + " (edited)"
	h.Type(" (edited)")
	h.Key(shell.KeyEnter)
	h.Step(0.016)

	if st.editing != -1 {
		t.Errorf("editor still open after Enter (editing=%d)", st.editing)
	}
	if st.items[0].text != want {
		t.Fatalf("items[0].text = %q, want %q", st.items[0].text, want)
	}
}

// Clicking the page background ends an open edit. gophics leaves focus alone
// when a press lands on nothing focusable, so without the page handling it the
// row stays an editor forever.
func TestClickingAwayClosesTheEditor(t *testing.T) {
	h, st := newHeadless(t)
	p := rowPoint(t, h, st, 0)

	opened := openEditor(h, st, p)
	if !opened {
		t.Fatal("editor did not open")
	}

	h.Tap(geom.Pt{X: 220, Y: 520}) // empty space below the list
	h.Step(0.016)
	if st.editing != -1 {
		t.Fatalf("editing = %d after clicking away, want -1", st.editing)
	}
}

// openEditor double-clicks row 0's label. rowPoint finds the row by hovering
// at x=220, which is past these short labels and lands on the row background;
// the label is inset by the padding and checkbox and sits mid-row, so the
// exact point has to be searched for rather than assumed. Probing on
// st.editing keeps a miss harmless — a stray Enter into the top field would
// add a todo instead.
func openEditor(h *app.Headless, st *todoState, p geom.Pt) bool {
	for dy := float32(0); dy <= 30; dy += 6 {
		for x := float32(40); x < 200; x += 8 {
			q := geom.Pt{X: x, Y: p.Y + dy}
			h.Tap(q)
			h.Step(0.05) // still inside the double-tap window
			h.Tap(q)
			h.Step(0.016)
			if st.editing == 0 {
				return true
			}
			if st.editing != -1 {
				st.editing = -1 // opened the wrong row; reset and keep looking
			}
		}
	}
	return false
}
