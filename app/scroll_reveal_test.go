package app

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// editorApp is a multiline TextField inside a short Scroll — the notes-editor
// shape — used to verify the caret scrolls into view as you type past the fold.
type editorApp struct {
	ctrl *widget.ScrollController
	hook func(*editorState)
}

func (a editorApp) CreateState() widget.State { return &editorState{ctrl: a.ctrl, hook: a.hook} }

type editorState struct {
	widget.StateBase[editorApp]
	ctrl *widget.ScrollController
	hook func(*editorState)
	text string
}

func (s *editorState) Init(widget.Ctx) {
	if s.hook != nil {
		s.hook(s)
	}
}

func (s *editorState) Build(widget.Ctx) widget.Widget {
	return widget.Scroll{
		Controller: s.ctrl,
		Child: widget.TextField{
			Value:     s.text,
			Multiline: true,
			Size:      14,
			OnChange:  func(t string) { s.SetState(func() { s.text = t }) },
		},
	}
}

func TestCaretScrollsIntoViewWhileTyping(t *testing.T) {
	var ctrl widget.ScrollController
	h, err := NewHeadless(editorApp{ctrl: &ctrl},
		// A short viewport so a handful of lines overflows it.
		Config{Size: geom.Size{W: 240, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Focus the field and type a first line.
	h.Tap(geom.Pt{X: 20, Y: 10})
	h.Type("first line")
	h.Render()
	if off := ctrl.Offset(); off != 0 {
		t.Fatalf("a single visible line should not scroll: offset=%v", off)
	}

	// Add many lines, past the bottom of the viewport. Each Enter should keep
	// the caret in view, so the offset grows.
	for i := 0; i < 20; i++ {
		h.Key(shell.KeyEnter)
		h.Type("line")
		h.Render()
	}
	off := ctrl.Offset()
	if off <= 0 {
		t.Fatalf("caret past the fold did not scroll into view: offset=%v", off)
	}
	max := ctrl.MaxOffset()
	if off < max-1 {
		t.Fatalf("caret at the end should scroll near the bottom: offset=%v max=%v", off, max)
	}

	// Now move the caret back to the top with Home-ish navigation (many Ups):
	// the view should follow the caret back up.
	for i := 0; i < 30; i++ {
		h.Key(shell.KeyUp)
		h.Render()
	}
	if up := ctrl.Offset(); up >= off {
		t.Fatalf("caret moving up did not scroll back up: offset=%v (was %v)", up, off)
	}
}

// sanity: the keys actually reach the field (guards the offset test above).
func TestCaretRevealTextGrew(t *testing.T) {
	var st *editorState
	h, err := NewHeadless(editorApp{hook: func(s *editorState) { st = s }},
		Config{Size: geom.Size{W: 240, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 20, Y: 10})
	for i := 0; i < 5; i++ {
		h.Type("x")
		h.Key(shell.KeyEnter)
		h.Render()
	}
	if got := strings.Count(st.text, "\n"); got < 4 {
		t.Fatalf("keys did not reach the field (newlines=%d, text=%q)", got, st.text)
	}
}
