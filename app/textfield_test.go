package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

type fieldApp struct{ hook func(*fieldAppState) }

func (a fieldApp) CreateState() widget.State { return &fieldAppState{hook: a.hook} }

type fieldAppState struct {
	widget.StateBase[fieldApp]
	hook      func(*fieldAppState)
	value     string
	submitted []string
}

func (s *fieldAppState) Init(widget.Ctx) { s.hook(s) }

func (s *fieldAppState) Build(widget.Ctx) widget.Widget {
	return widget.Padding{All: 10, Child: widget.TextField{
		Value:          s.value,
		OnChange:       func(v string) { s.SetState(func() { s.value = v }) },
		OnSubmit:       func(v string) { s.SetState(func() { s.submitted = append(s.submitted, v); s.value = "" }) },
		TextColor:      paint.RGB(1, 1, 1),
		CaretColor:     paint.RGB(0.3, 0.6, 1),
		SelectionColor: paint.Color{R: 0.3, G: 0.6, B: 1, A: 0.4},
	}}
}

func fieldHarness(t *testing.T) (*Headless, *fieldAppState) {
	t.Helper()
	var st *fieldAppState
	h, err := NewHeadless(fieldApp{hook: func(s *fieldAppState) { st = s }}, Config{
		Size: geom.Size{W: 300, H: 60}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func TestFieldTypingAndSubmit(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("hello")
	if st.value != "hello" {
		t.Fatalf("value = %q", st.value)
	}
	h.Key(shell.KeyEnter)
	if len(st.submitted) != 1 || st.submitted[0] != "hello" || st.value != "" {
		t.Fatalf("submitted=%v value=%q", st.submitted, st.value)
	}
	h.Type("next")
	if st.value != "next" {
		t.Fatalf("after clear, value = %q", st.value)
	}
}

func TestFieldCaretEditing(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("world")
	h.Key(shell.KeyHome)
	h.Type("hello ")
	if st.value != "hello world" {
		t.Fatalf("value = %q", st.value)
	}
	// Left ×5 from end of "hello " insertion point, delete-forward one.
	h.Key(shell.KeyEnd)
	for range 5 {
		h.Key(shell.KeyLeft)
	}
	h.Key(shell.KeyDelete)
	if st.value != "hello orld" {
		t.Fatalf("value = %q", st.value)
	}
}

func TestFieldSelectionShortcuts(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("select me")

	h.KeyMod(shell.KeyA, shell.ModSuper) // select all
	h.KeyMod(shell.KeyC, shell.ModSuper) // copy
	clip := h.core.Owner.Clipboard.(*MemClipboard)
	if clip.S != "select me" {
		t.Fatalf("clipboard = %q", clip.S)
	}

	h.Type("x") // replaces selection
	if st.value != "x" {
		t.Fatalf("replace-all = %q", st.value)
	}

	h.KeyMod(shell.KeyA, shell.ModSuper)
	h.KeyMod(shell.KeyV, shell.ModSuper) // paste over selection
	if st.value != "select me" {
		t.Fatalf("paste = %q", st.value)
	}

	h.KeyMod(shell.KeyA, shell.ModSuper)
	h.KeyMod(shell.KeyX, shell.ModSuper) // cut
	if st.value != "" || clip.S != "select me" {
		t.Fatalf("cut: value=%q clip=%q", st.value, clip.S)
	}
}

func TestFieldShiftArrowSelection(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("abcdef")
	h.Key(shell.KeyHome)
	for range 3 {
		h.KeyMod(shell.KeyRight, shell.ModShift)
	}
	h.Key(shell.KeyBackspace) // deletes selection "abc"
	if st.value != "def" {
		t.Fatalf("value = %q", st.value)
	}
}

func TestFieldClickPlacesCaret(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("mmmm")
	// Click near the left edge of the text: caret to index 0; then type.
	h.Tap(geom.Pt{X: 11, Y: 30})
	h.Type("Z")
	if st.value != "Zmmmm" {
		t.Fatalf("value = %q, want caret at start", st.value)
	}
	// Click far right (inside the field, past the text): caret to end.
	h.Tap(geom.Pt{X: 285, Y: 30})
	h.Type("Q")
	if st.value != "ZmmmmQ" {
		t.Fatalf("value = %q, want caret at end", st.value)
	}
}

func TestFieldDragSelects(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("abcdef")
	w := h.core.Painter.MeasureWidthIn("", "abcdef", 14)
	// Drag from before text start to past its end: selects everything.
	h.DragTo(geom.Pt{X: 10, Y: 30}, geom.Pt{X: 12 + w, Y: 30})
	h.Release(geom.Pt{X: 12 + w, Y: 30})
	h.Type("R") // replaces selection
	if st.value != "R" {
		t.Fatalf("drag-select replace: value = %q", st.value)
	}
}

func TestFieldIMEComposition(t *testing.T) {
	h, st := fieldHarness(t)
	h.Type("ab")
	h.Key(shell.KeyLeft) // caret between a and b

	h.Compose("ni", 2)
	if st.value != "ab" {
		t.Fatalf("preedit must not change the value: %q", st.value)
	}
	h.Compose("nih", 3)
	h.CommitComposition("你")
	if st.value != "a你b" {
		t.Fatalf("committed at caret: %q", st.value)
	}

	// Cancelled composition (empty commit) leaves the value alone.
	h.Compose("x", 1)
	h.CommitComposition("")
	if st.value != "a你b" {
		t.Fatalf("cancelled composition changed value: %q", st.value)
	}
}
