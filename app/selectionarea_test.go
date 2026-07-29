package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// selAreaApp wraps a two-line column of plain Text in a SelectionArea, so the
// test exercises cross-fragment selection over widgets that are not themselves
// SelectableText.
type selAreaApp struct{ a, b string }

func (a selAreaApp) CreateState() widget.State { return &selAreaState{a: a.a, b: a.b} }

type selAreaState struct {
	widget.StateBase[selAreaApp]
	a, b string
}

func (s *selAreaState) Build(widget.Ctx) widget.Widget {
	col := widget.Column(
		widget.Text{S: s.a, Size: 14},
		widget.Text{S: s.b, Size: 14},
	)
	col.CrossAlign = layout.CrossStart
	return widget.SelectionArea{Child: col}
}

func selAreaHarness(t *testing.T, a, b string) *Headless {
	t.Helper()
	h, err := NewHeadless(selAreaApp{a: a, b: b},
		Config{Size: geom.Size{W: 400, H: 120}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

// TestSelectionAreaCopyAcrossFragments drags from the first line through the
// second and copies — the plain Text widgets should behave as one selection.
func TestSelectionAreaCopyAcrossFragments(t *testing.T) {
	h := selAreaHarness(t, "Hello", "World")
	h.Render()

	// Press at the start of line 1, drag past the end of line 2.
	h.DragTo(geom.Pt{X: 1, Y: 6}, geom.Pt{X: 3000, Y: 26})
	h.Release(geom.Pt{X: 3000, Y: 26})
	h.Render()

	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got, want := clip(h), "Hello\nWorld"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// listSelApp mirrors the HN structure: a SelectionArea wrapping a scrolling
// LazyList, to catch gesture/context issues that a plain Column would miss.
type listSelApp struct{ items []string }

func (a listSelApp) CreateState() widget.State { return &listSelState{items: a.items} }

type listSelState struct {
	widget.StateBase[listSelApp]
	items []string
}

func (s *listSelState) Build(widget.Ctx) widget.Widget {
	return widget.SelectionArea{Child: widget.LazyList{
		Count:           len(s.items),
		EstimatedExtent: 20,
		Build:           func(i int) widget.Widget { return widget.Text{S: s.items[i], Size: 14} },
	}}
}

// TestSelectionAreaInLazyList checks a horizontal drag selects a list item's
// text (a vertical scroll must not steal the horizontal drag).
func TestSelectionAreaInLazyList(t *testing.T) {
	h, err := NewHeadless(listSelApp{items: []string{"alpha", "beta", "gamma"}},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	// Horizontal drag across the first item.
	h.DragTo(geom.Pt{X: 1, Y: 8}, geom.Pt{X: 3000, Y: 8})
	h.Release(geom.Pt{X: 3000, Y: 8})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got, want := clip(h), "alpha"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// TestSelectionAreaSingleFragment selects within just the first line.
func TestSelectionAreaSingleFragment(t *testing.T) {
	h := selAreaHarness(t, "Hello", "World")
	h.Render()
	h.DragTo(geom.Pt{X: 1, Y: 6}, geom.Pt{X: 3000, Y: 6})
	h.Release(geom.Pt{X: 3000, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got, want := clip(h), "Hello"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}
