package app

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// nativeFieldApp mounts one configurable TextField, so a test can say which
// platform's conventions it is under and which options are set.
type nativeFieldApp struct {
	cfg  widget.TextField
	hook func(*nativeFieldState)
}

func (a nativeFieldApp) CreateState() widget.State { return &nativeFieldState{hook: a.hook} }

type nativeFieldState struct {
	widget.StateBase[nativeFieldApp]
	hook   func(*nativeFieldState)
	value  string
	inited bool
}

func (s *nativeFieldState) Init(widget.Ctx) { s.hook(s) }

func (s *nativeFieldState) Build(widget.Ctx) widget.Widget {
	f := s.W().cfg
	if !s.inited {
		s.value, s.inited = f.Value, true
	}
	f.Value = s.value
	f.OnChange = func(v string) { s.SetState(func() { s.value = v }) }
	f.TextColor = paint.RGB(1, 1, 1)
	return widget.Padding{All: 10, Child: f}
}

// nativeField mounts cfg under the given platform conventions, focused, with
// the caret at the end of the value.
func nativeField(t *testing.T, cfg widget.TextField, g shell.GestureTuning) (*Headless, *nativeFieldState) {
	t.Helper()
	var st *nativeFieldState
	h, err := NewHeadless(nativeFieldApp{cfg: cfg, hook: func(s *nativeFieldState) { st = s }}, Config{
		Size: geom.Size{W: 400, H: 60}, Font: goregular.TTF, Gestures: g,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 380, Y: 30}) // focus, caret at the end
	h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05)
	h.Render()
	return h, st
}

var (
	macKeys = shell.GestureTuning{MacKeys: true}
	pcKeys  = shell.GestureTuning{}
)

func key(h *Headless, code shell.KeyCode, mods shell.Mods) {
	h.KeyMod(code, mods)
	h.Render()
}

// Word deletion is Alt+Backspace on a Mac and Ctrl+Backspace elsewhere; the
// other platform's chord must not do it, or a Mac user pressing Ctrl+Backspace
// (which Cocoa treats as plain backspace) would lose a word.
func TestWordDeleteFollowsPlatformConvention(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "alpha beta gamma"}, macKeys)
	key(h, shell.KeyBackspace, shell.ModAlt)
	if st.value != "alpha beta " {
		t.Errorf("mac Alt+Backspace: %q", st.value)
	}
	key(h, shell.KeyBackspace, shell.ModCtrl) // Emacs Ctrl+H is a single backspace
	if st.value != "alpha beta" {
		t.Errorf("mac Ctrl+Backspace should delete one character: %q", st.value)
	}

	h, st = nativeField(t, widget.TextField{Value: "alpha beta gamma"}, pcKeys)
	key(h, shell.KeyBackspace, shell.ModCtrl)
	if st.value != "alpha beta " {
		t.Errorf("pc Ctrl+Backspace: %q", st.value)
	}
	key(h, shell.KeyBackspace, shell.ModAlt)
	if st.value != "alpha beta" {
		t.Errorf("pc Alt+Backspace is not a word delete: %q", st.value)
	}
}

// Cmd+Left on a Mac is the line start; Ctrl+Left is a word on a PC.
func TestLineAndWordMovement(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "one two"}, macKeys)
	key(h, shell.KeyLeft, shell.ModSuper)
	h.Type("X")
	if st.value != "Xone two" {
		t.Errorf("mac Cmd+Left then type: %q", st.value)
	}
	h, st = nativeField(t, widget.TextField{Value: "one two"}, pcKeys)
	key(h, shell.KeyLeft, shell.ModCtrl)
	h.Type("X")
	if st.value != "one Xtwo" {
		t.Errorf("pc Ctrl+Left then type: %q", st.value)
	}
}

// Ctrl+K on a Mac kills to the end of the line — the Cocoa binding every
// native field honors, and the one that must not be "select all" (Ctrl+A).
func TestMacEmacsBindings(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "keep this|kill this"}, macKeys)
	key(h, shell.KeyA, shell.ModCtrl) // line start, not select-all
	for i := 0; i < len("keep this|"); i++ {
		key(h, shell.KeyF, shell.ModCtrl) // forward one
	}
	key(h, shell.KeyK, shell.ModCtrl)
	if st.value != "keep this|" {
		t.Errorf("after Ctrl+A, Ctrl+F×n, Ctrl+K: %q", st.value)
	}
}

// Undo takes back a typed run as one step and redo restores it — Cmd+Z and
// Shift+Cmd+Z on a Mac, Ctrl+Z and Ctrl+Y on a PC.
func TestUndoRedoKeys(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: ""}, macKeys)
	h.Type("hello")
	h.Render()
	key(h, shell.KeyZ, shell.ModSuper)
	if st.value != "" {
		t.Errorf("mac Cmd+Z should take back the typed run: %q", st.value)
	}
	key(h, shell.KeyZ, shell.ModSuper|shell.ModShift)
	if st.value != "hello" {
		t.Errorf("mac Shift+Cmd+Z should redo: %q", st.value)
	}

	h, st = nativeField(t, widget.TextField{Value: ""}, pcKeys)
	h.Type("hello")
	h.Render()
	key(h, shell.KeyZ, shell.ModCtrl)
	if st.value != "" {
		t.Errorf("pc Ctrl+Z: %q", st.value)
	}
	key(h, shell.KeyY, shell.ModCtrl)
	if st.value != "hello" {
		t.Errorf("pc Ctrl+Y should redo: %q", st.value)
	}
}

// A password field shows bullets, tells assistive technology bullets, and
// never puts its content on the clipboard.
func TestObscureMasksAndRefusesCopy(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "secret", Obscure: true}, pcKeys)
	var sem []string
	for _, n := range h.Semantics() {
		sem = append(sem, n.Value)
	}
	joined := strings.Join(sem, "|")
	if strings.Contains(joined, "secret") {
		t.Errorf("semantics leaked the password: %q", joined)
	}
	if !strings.Contains(joined, "••••••") {
		t.Errorf("semantics should report bullets, got %q", joined)
	}
	key(h, shell.KeyA, shell.ModCtrl)
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "" {
		t.Errorf("copy from a password field put %q on the clipboard", got)
	}
}

// Read-only accepts the caret and selection but no edits, and disabled
// accepts nothing.
func TestReadOnlyAndDisabled(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "fixed", ReadOnly: true}, pcKeys)
	h.Type("x")
	key(h, shell.KeyBackspace, 0)
	if st.value != "fixed" {
		t.Errorf("read-only field changed to %q", st.value)
	}
	key(h, shell.KeyA, shell.ModCtrl)
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "fixed" {
		t.Errorf("read-only should still copy; clipboard %q", got)
	}
	h, st = nativeField(t, widget.TextField{Value: "off", Disabled: true}, pcKeys)
	h.Type("x")
	if st.value != "off" {
		t.Errorf("disabled field changed to %q", st.value)
	}
}

// MaxLength truncates what would overflow, whether typed or pasted.
func TestMaxLengthTruncates(t *testing.T) {
	h, st := nativeField(t, widget.TextField{Value: "ab", MaxLength: 4}, pcKeys)
	h.Type("cdef")
	h.Render()
	if st.value != "abcd" {
		t.Errorf("typing past the limit: %q", st.value)
	}
	h.Clipboard().S = "XYZ"
	key(h, shell.KeyA, shell.ModCtrl)
	key(h, shell.KeyV, shell.ModCtrl)
	if st.value != "XYZ" {
		t.Errorf("pasting over a selection within the limit: %q", st.value)
	}
	h.Clipboard().S = "123456"
	key(h, shell.KeyA, shell.ModCtrl)
	key(h, shell.KeyV, shell.ModCtrl)
	if st.value != "1234" {
		t.Errorf("pasting past the limit: %q", st.value)
	}
}

// Shift-click extends the selection from the caret; a triple click selects
// everything.
func TestShiftClickAndTripleClick(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "one two three"}, pcKeys)
	// Caret is at the end; shift-click near the start selects back to it.
	// Hold shift: the modifier state the field reads at press time comes
	// from the last key event's Mods.
	h.core.Keyboard(shell.Key{Kind: shell.KeyPress, Code: shell.KeyShift, Mods: shell.ModShift})
	h.Tap(geom.Pt{X: 14, Y: 30})
	h.core.Keyboard(shell.Key{Kind: shell.KeyRelease, Code: shell.KeyShift})
	h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05)
	h.Render()
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; !strings.HasSuffix(got, "three") || len(got) < 8 {
		t.Errorf("shift-click selection copied %q, want a span reaching the end", got)
	}

	h, _ = nativeField(t, widget.TextField{Value: "one two three"}, pcKeys)
	for i := 0; i < 3; i++ {
		h.Tap(geom.Pt{X: 40, Y: 30})
		h.Step(0.05)
	}
	h.Render()
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "one two three" {
		t.Errorf("triple click copied %q, want the whole value", got)
	}
}

// A right click selects the word under the pointer and opens the edit menu.
func TestRightClickOpensEditMenuOnWord(t *testing.T) {
	h, _ := nativeField(t, widget.TextField{Value: "alpha beta"}, pcKeys)
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 20, Y: 30}, Button: 1})
	h.Render()
	var labels []string
	for _, n := range h.Semantics() {
		labels = append(labels, n.Label)
	}
	all := strings.Join(labels, "|")
	if !strings.Contains(all, "Copy") {
		t.Errorf("no edit menu after a right click; semantics: %q", all)
	}
	key(h, shell.KeyC, shell.ModCtrl)
	if got := h.Clipboard().S; got != "alpha" {
		t.Errorf("right click should have selected the word under it; copied %q", got)
	}
}
