package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

type selApp struct{ text string }

func (a selApp) CreateState() widget.State { return &selAppState{text: a.text} }

type selAppState struct {
	widget.StateBase[selApp]
	text string
}

func (s *selAppState) Build(widget.Ctx) widget.Widget {
	return widget.SelectableText{S: s.text, Size: 14}
}

func selHarness(t *testing.T, text string) *Headless {
	t.Helper()
	h, err := NewHeadless(selApp{text: text},
		Config{Size: geom.Size{W: 400, H: 120}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func clip(h *Headless) string {
	return h.Core.Owner.Clipboard.(*MemClipboard).S
}

func TestSelectableCopyAll(t *testing.T) {
	h := selHarness(t, "Hello World")
	h.Render()

	// Press at the start, drag past the end to select everything.
	h.DragTo(geom.Pt{X: 1, Y: 8}, geom.Pt{X: 3000, Y: 8})
	h.Release(geom.Pt{X: 3000, Y: 8})
	h.Render()

	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "Hello World" {
		t.Fatalf("copied %q, want %q", got, "Hello World")
	}
}

func TestSelectableCopyRequiresSelection(t *testing.T) {
	h := selHarness(t, "Hello World")
	h.Render()

	// A plain click (press+release, no drag) makes an empty selection.
	h.Tap(geom.Pt{X: 20, Y: 8})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "" {
		t.Fatalf("empty selection copied %q, want nothing", got)
	}
}

func TestSelectableCtrlCAlsoCopies(t *testing.T) {
	h := selHarness(t, "copy me")
	h.Render()
	h.DragTo(geom.Pt{X: 1, Y: 8}, geom.Pt{X: 3000, Y: 8})
	h.Release(geom.Pt{X: 3000, Y: 8})
	h.Render()

	// Ctrl+C (non-mac) works through Mods.Command() too.
	h.KeyMod(shell.KeyC, shell.ModCtrl)
	if got := clip(h); got != "copy me" {
		t.Fatalf("ctrl+c copied %q, want %q", got, "copy me")
	}
}
