package app

import (
	"bytes"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// obscureApp is a controlled TextField, as an app would use one: Value is
// state, OnChange writes it back and records what the field reported.
type obscureApp struct {
	obscure bool
	initial string
	changes *[]string
}

func (a obscureApp) CreateState() widget.State {
	return &obscureState{value: a.initial}
}

type obscureState struct {
	widget.StateBase[obscureApp]
	value string
}

func (s *obscureState) Build(widget.Ctx) widget.Widget {
	a := s.W()
	return widget.TextField{
		// Typed straight into, so it opens focused; a field does not take
		// focus on mount by itself.
		Autofocus: true,
		Value:     s.value,
		Obscure:   a.obscure,
		// An explicit colour: the default is black, and so is an unset
		// Background, so the pixel test below would compare empty frames.
		TextColor: paint.RGB(0.2, 0.4, 0.9),
		OnChange: func(v string) {
			if a.changes != nil {
				*a.changes = append(*a.changes, v)
			}
			s.SetState(func() { s.value = v })
		},
	}
}

func obscureHarness(t *testing.T, a obscureApp) *Headless {
	t.Helper()
	h, err := NewHeadless(a, Config{Size: geom.Size{W: 240, H: 40}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func fieldNode(t *testing.T, h *Headless) layout.SemNode {
	t.Helper()
	h.Render()
	// The Interactive wrapper is also a textfield node; the one carrying the
	// value is the field box itself, so take the node that has one.
	var found *layout.SemNode
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Role == layout.RoleTextField && n.Value != "" {
			n := n
			found = &n
		}
	}
	if found == nil {
		t.Fatal("no textfield node with a value in the semantics tree")
	}
	return *found
}

// What the user typed reaches the app in the clear; what reaches the
// semantics tree — and so a screen reader or a test harness — is bullets, one
// per rune, and the node is flagged secure.
func TestObscureFieldMasksSemanticsButNotOnChange(t *testing.T) {
	var changes []string
	h := obscureHarness(t, obscureApp{obscure: true, changes: &changes})

	h.Type("héllo") // one multi-byte rune, so a byte count would show
	if len(changes) == 0 || changes[len(changes)-1] != "héllo" {
		t.Fatalf("OnChange reported %q, want the real text %q", changes, "héllo")
	}

	n := fieldNode(t, h)
	if n.Value != "•••••" {
		t.Errorf("semantics Value = %q, want five bullets — the secret is readable from the tree", n.Value)
	}
	if !n.Secure {
		t.Error("semantics node is not marked Secure; the AT would announce it as a plain field")
	}
}

// The caret moves through the real text one rune at a time, so editing in the
// middle of a masked value lands where the bullets say it will.
func TestObscureFieldCaretMovesByRune(t *testing.T) {
	var changes []string
	h := obscureHarness(t, obscureApp{obscure: true, changes: &changes})

	h.Type("héllo")
	h.Key(shell.KeyLeft)
	h.Key(shell.KeyLeft)
	h.Type("X")
	if got := changes[len(changes)-1]; got != "hélXlo" {
		t.Fatalf("after two Lefts and an insert the value is %q, want %q", got, "hélXlo")
	}
	if n := fieldNode(t, h); n.Value != "••••••" {
		t.Errorf("semantics Value = %q, want six bullets", n.Value)
	}
}

// Copy and Cut leave the clipboard alone and the text intact; Paste still
// works, because putting a secret in leaks nothing.
func TestObscureFieldDoesNotCopyOrCut(t *testing.T) {
	var changes []string
	h := obscureHarness(t, obscureApp{obscure: true, changes: &changes})
	h.Clipboard().S = "untouched"

	h.Type("secret")
	h.KeyMod(shell.KeyA, shell.ModSuper)
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := h.Clipboard().S; got != "untouched" {
		t.Fatalf("Copy put %q on the clipboard; a masked field must not", got)
	}

	before := len(changes)
	h.KeyMod(shell.KeyX, shell.ModSuper)
	if got := h.Clipboard().S; got != "untouched" {
		t.Fatalf("Cut put %q on the clipboard", got)
	}
	if len(changes) != before {
		t.Fatalf("Cut changed the text to %q; it must be a no-op", changes[len(changes)-1])
	}

	// Paste replaces the (still selected) text.
	h.Clipboard().S = "hunter2"
	h.KeyMod(shell.KeyV, shell.ModSuper)
	if got := changes[len(changes)-1]; got != "hunter2" {
		t.Fatalf("Paste produced %q, want %q — paste must keep working", got, "hunter2")
	}

	// And the same field without Obscure copies, so the test is about the
	// flag and not about a broken clipboard.
	plain := obscureHarness(t, obscureApp{initial: "visible"})
	plain.KeyMod(shell.KeyA, shell.ModSuper)
	plain.KeyMod(shell.KeyC, shell.ModSuper)
	if got := plain.Clipboard().S; got != "visible" {
		t.Fatalf("plain field copied %q, want %q", got, "visible")
	}
}

// Two obscured fields holding different secrets of the same length paint the
// same pixels — the frame carries no more than the length — while the same
// field un-obscured paints something else.
func TestObscureFieldRendersDots(t *testing.T) {
	encode := func(h *Headless) []byte {
		var buf bytes.Buffer
		if err := png.Encode(&buf, h.Render()); err != nil {
			panic(err)
		}
		return buf.Bytes()
	}
	a := encode(obscureHarness(t, obscureApp{obscure: true, initial: "abcdef"}))
	b := encode(obscureHarness(t, obscureApp{obscure: true, initial: "xyzw12"}))
	if !bytes.Equal(a, b) {
		t.Error("two obscured values of equal length rendered differently; the glyphs leak the text")
	}
	c := encode(obscureHarness(t, obscureApp{initial: "abcdef"}))
	if bytes.Equal(a, c) {
		t.Error("an obscured field rendered identically to a plain one; nothing was masked")
	}
}

// A double tap in an Obscure field selects everything, not the word under
// the tap: a word highlight over the bullets would show where the secret's
// words begin and end. The plain field is the control that the double tap
// itself selects a word.
func TestObscureFieldDoubleTapSelectsAll(t *testing.T) {
	var changes []string
	h := obscureHarness(t, obscureApp{obscure: true, initial: "two words", changes: &changes})
	doubleTapAt(h, 12, 20) // inside the first word
	h.Type("Z")
	if got := changes[len(changes)-1]; got != "Z" {
		t.Fatalf("typing after a double tap left %q, want %q: the double tap selected a word, not all", got, "Z")
	}

	var plainChanges []string
	plain := obscureHarness(t, obscureApp{initial: "two words", changes: &plainChanges})
	doubleTapAt(plain, 12, 20)
	plain.Type("Z")
	if got := plainChanges[len(plainChanges)-1]; got != "Z words" {
		t.Fatalf("plain field: typing after a double tap left %q, want %q (word selection)", got, "Z words")
	}
}
