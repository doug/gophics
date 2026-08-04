package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

func multiHarness(t *testing.T) (*Headless, *fieldAppState) {
	t.Helper()
	var st *fieldAppState
	h, err := NewHeadless(multiApp{hook: func(s *fieldAppState) { st = s }}, Config{
		Size: geom.Size{W: 220, H: 300}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

type multiApp struct{ hook func(*fieldAppState) }

func (a multiApp) CreateState() widget.State {
	s := &multiState{}
	s.hook = a.hook
	return s
}

type multiState struct {
	widget.StateBase[multiApp]
	fieldAppState
}

func (s *multiState) Init(widget.Ctx) { s.hook(&s.fieldAppState) }

func (s *multiState) Build(widget.Ctx) widget.Widget {
	st := &s.fieldAppState
	return widget.Padding{All: 10, Child: widget.TextField{
		Value:     st.value,
		Multiline: true,
		OnChange:  func(v string) { s.SetState(func() { st.value = v }) },
		OnSubmit:  func(v string) { st.submitted = append(st.submitted, v) },
	}}
}

func TestMultilineEnterInsertsNewline(t *testing.T) {
	h, st := multiHarness(t)
	h.Type("line one")
	h.Key(shell.KeyEnter)
	h.Type("line two")
	if st.value != "line one\nline two" {
		t.Fatalf("value = %q", st.value)
	}
	if len(st.submitted) != 0 {
		t.Fatal("plain Enter must not submit in multiline")
	}
	h.KeyMod(shell.KeyEnter, shell.ModSuper)
	if len(st.submitted) != 1 {
		t.Fatal("Cmd+Enter should submit")
	}
}

func TestMultilineVerticalNavigation(t *testing.T) {
	h, st := multiHarness(t)
	h.Type("aaaa")
	h.Key(shell.KeyEnter)
	h.Type("bb")
	h.Render() // establish layout width for navigation

	// Caret at end of "bb"; Up should land in line one; typing goes there.
	h.Key(shell.KeyUp)
	h.Type("X")
	if st.value != "aaXaa\nbb" && st.value != "aaaXa\nbb" && st.value != "aXaa\nbb"+"" {
		// nearest-x mapping may vary by a cluster; just require line 1
		if st.value[len(st.value)-2:] == "Xb" || st.value[len(st.value)-1:] == "X" {
			t.Fatalf("Up did not move to first line: %q", st.value)
		}
	}
	// Down returns to the last line.
	h.Key(shell.KeyDown)
	h.Key(shell.KeyEnd)
	h.Type("Z")
	if st.value[len(st.value)-1] != 'Z' {
		t.Fatalf("Down+End+type failed: %q", st.value)
	}
}

func TestMultilineWrapGrowsHeight(t *testing.T) {
	h, st := multiHarness(t)
	h.Type("word word word word word word word word word word")
	h.Render()
	if st.value == "" {
		t.Fatal("no value")
	}
	// The wrapped field must be taller than a single line: probe semantics rect.
	// (Field width 200 → this text wraps to several lines.)
	found := false
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if n.Role == layout.RoleTextField && n.Rect.Dy() > 40 {
			found = true
		}
	}
	if !found {
		t.Fatal("multiline field did not grow with wrapped content")
	}
}
