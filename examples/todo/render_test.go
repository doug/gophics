package main

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
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

// fieldPoint is the centre of the first text field in the semantics tree.
func fieldPoint(t *testing.T, h *app.Headless) geom.Pt {
	t.Helper()
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Role == layout.RoleTextField {
			return geom.Pt{X: n.Rect.Min.X + n.Rect.Dx()/2, Y: n.Rect.Min.Y + n.Rect.Dy()/2}
		}
	}
	t.Fatal("no text field in the semantics tree")
	return geom.Pt{}
}

func TestTypingAddsItem(t *testing.T) {
	h, st := newHeadless(t)
	n := len(st.items)

	// The add field does not focus on mount — that would open the app with
	// the keyboard up on a phone — so the user taps it first.
	h.Tap(fieldPoint(t, h))
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
	return openEditorAt(h, st, p, 0)
}

// openEditorOn double-clicks row i's label, locating the row by hover first.
func openEditorOn(t *testing.T, h *app.Headless, st *todoState, i int) bool {
	t.Helper()
	return openEditorAt(h, st, rowPoint(t, h, st, i), i)
}

func openEditorAt(h *app.Headless, st *todoState, p geom.Pt, row int) bool {
	for dy := float32(0); dy <= 30; dy += 6 {
		for x := float32(40); x < 200; x += 8 {
			q := geom.Pt{X: x, Y: p.Y + dy}
			h.Tap(q)
			h.Step(0.05) // still inside the double-tap window
			h.Tap(q)
			h.Step(0.016)
			if st.editing == row {
				return true
			}
			if st.editing != -1 {
				st.editing = -1 // opened the wrong row; reset and keep looking
			}
		}
	}
	return false
}

// Deleting a row while a later one is being edited must not move the editor.
// editing is an index, and the rows above it shift when one is removed; before
// remove adjusted it, the draft was committed into whichever todo slid into
// the edited slot — the user lost their edit and overwrote a neighbour.
func TestDeletingAnotherRowKeepsTheEdit(t *testing.T) {
	h, st := newHeadless(t)
	before := texts(st)

	if !openEditorOn(t, h, st, 2) {
		t.Fatal("editor did not open on row 2")
	}
	h.Type(" (edited)")
	want := before[2] + " (edited)"
	if st.draft != want {
		t.Fatalf("draft = %q, want %q", st.draft, want)
	}

	// Hover row 0 so its × appears, then tap the × where the semantics tree
	// says it is. A miss would land on the row itself, and a row tap toggles
	// and commits — so this has to be exact rather than a scan. Render, not
	// Step: the rect is recorded by the paint that follows the hover rebuild.
	rowPoint(t, h, st, 0)
	h.Render()
	var del *layout.SemNode
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Label == "×" {
			del = &n
			break
		}
	}
	if del == nil || del.Rect.IsEmpty() {
		t.Fatal("hovering row 0 did not show its × button")
	}
	n := len(st.items)
	h.Tap(geom.Pt{X: del.Rect.Min.X + del.Rect.Dx()/2, Y: del.Rect.Min.Y + del.Rect.Dy()/2})
	h.Step(0.016)
	if len(st.items) != n-1 {
		t.Fatal("tapping × on row 0 did not delete it")
	}
	if st.editing != 1 {
		t.Fatalf("editing = %d after deleting row 0, want 1 (the edited row moved up)", st.editing)
	}

	h.Tap(geom.Pt{X: 220, Y: 520}) // click away: commits
	h.Step(0.016)

	got := texts(st)
	exp := append([]string{before[1], want}, before[3:]...)
	if strings.Join(got, "|") != strings.Join(exp, "|") {
		t.Fatalf("items after delete + commit:\n got %q\nwant %q", got, exp)
	}
}

// remove's editing adjustment on the cases the headless test does not reach:
// deleting the edited row itself, and deleting a row below it.
func TestRemoveKeepsEditorOnItsRow(t *testing.T) {
	cases := []struct {
		name        string
		editing, rm int
		wantEditing int
	}{
		{"row above the edit", 3, 1, 2},
		{"the edited row", 3, 3, -1},
		{"row below the edit", 1, 3, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &todoState{editing: c.editing, draft: "draft", hover: -1}
			for _, n := range []string{"a", "b", "c", "d", "e"} {
				s.items = append(s.items, item{text: n})
			}
			s.remove(c.rm)
			if s.editing != c.wantEditing {
				t.Fatalf("editing = %d, want %d", s.editing, c.wantEditing)
			}
			if c.wantEditing >= 0 && s.draft != "draft" {
				t.Fatal("draft was dropped although its row survived")
			}
			if c.wantEditing < 0 && s.draft != "" {
				t.Fatal("draft outlived its row")
			}
		})
	}
}

func texts(st *todoState) []string {
	out := make([]string, len(st.items))
	for i, it := range st.items {
		out[i] = it.text
	}
	return out
}
