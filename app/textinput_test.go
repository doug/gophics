package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

// ctrlField is a controlled TextField wrapper (Value tracks edits), mirroring
// how a real app drives the field — so typed input accumulates across the
// build that the shell runs between events.
type ctrlField struct {
	multiline bool
	out       *string
}

func (c ctrlField) CreateState() widget.State { return &ctrlFieldState{} }

type ctrlFieldState struct {
	widget.StateBase[ctrlField]
	val string
}

func (s *ctrlFieldState) Build(widget.Ctx) widget.Widget {
	return widget.TextField{
		Value:     s.val,
		Multiline: s.W().multiline,
		OnChange: func(t string) {
			s.SetState(func() { s.val = t })
			*s.W().out = t
		},
	}
}

func driveField(t *testing.T, multiline bool, do func(h *Headless)) string {
	t.Helper()
	var got string
	h, err := NewHeadless(ctrlField{multiline: multiline, out: &got}, Config{Size: geom.Size{W: 300, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 150, Y: 100}) // focus
	do(h)
	h.Render()
	return got
}

func TestTextInputSpacesAccumulate(t *testing.T) {
	got := driveField(t, true, func(h *Headless) { h.Type("a b c") })
	if got != "a b c" {
		t.Errorf("got %q, want \"a b c\" (spaces should insert)", got)
	}
}

func TestTabInsertsInMultilineOnly(t *testing.T) {
	multi := driveField(t, true, func(h *Headless) {
		h.Type("ab")
		h.Key(shell.KeyTab)
		h.Type("cd")
	})
	if multi != "ab\tcd" {
		t.Errorf("multiline Tab: got %q, want \"ab\\tcd\"", multi)
	}

	single := driveField(t, false, func(h *Headless) {
		h.Type("ab")
		h.Key(shell.KeyTab)
		h.Type("cd")
	})
	if single != "abcd" {
		t.Errorf("single-line Tab should be a no-op: got %q, want \"abcd\"", single)
	}
}
