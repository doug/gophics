package app

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
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

// nestedSelApp offsets the SelectionArea by padding (like the HN app nests it
// under a header + gutters), to exercise the origin/offset math that a
// root-level area doesn't.
type nestedSelApp struct{ text string }

func (a nestedSelApp) CreateState() widget.State { return &nestedSelState{text: a.text} }

type nestedSelState struct {
	widget.StateBase[nestedSelApp]
	text string
}

func (s *nestedSelState) Build(widget.Ctx) widget.Widget {
	return widget.Padding{All: 40, Child: widget.SelectionArea{
		Child: widget.Text{S: s.text, Size: 14},
	}}
}

// TestSelectionAreaNestedOffset drags across text that starts 40px in, so the
// registry origin and pointer coords must both account for the offset.
func TestSelectionAreaNestedOffset(t *testing.T) {
	h, err := NewHeadless(nestedSelApp{text: "offset me"},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	// Text baseline sits ~40+ from the top; drag across it there.
	h.DragTo(geom.Pt{X: 41, Y: 48}, geom.Pt{X: 3000, Y: 48})
	h.Release(geom.Pt{X: 3000, Y: 48})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got, want := clip(h), "offset me"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// TestSelectionAreaVerticalDragOverScroll checks that a vertical drag which
// begins on text extends the selection down across items instead of being
// claimed by the list's vertical scroll (the DragPriority path — the (a) fix).
func TestSelectionAreaVerticalDragOverScroll(t *testing.T) {
	h, err := NewHeadless(listSelApp{items: []string{"alpha", "beta", "gamma", "delta"}},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	// Press on the first item's text, drag straight DOWN past the third item.
	// Items are ~20px tall: item 0 ~y8, item 2 ~y48.
	h.DragTo(geom.Pt{X: 1, Y: 6}, geom.Pt{X: 3000, Y: 50})
	h.Release(geom.Pt{X: 3000, Y: 50})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	// Should span multiple items — at minimum reach "gamma" — rather than
	// scrolling and selecting nothing beyond "alpha".
	got := clip(h)
	if got == "" || got == "alpha" {
		t.Fatalf("vertical drag on text did not extend selection across items: copied %q", got)
	}
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "gamma") {
		t.Fatalf("selection should span alpha..gamma; copied %q", got)
	}
}

// touchDown/touchMove dispatch touch-sourced pointer events (the harness
// helpers default to a mouse).
func touchDown(h *Headless, p geom.Pt) {
	h.core.Layout(geom.Size{W: 400, H: 200})
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: p, Source: shell.SourceTouch})
}
func touchMove(h *Headless, p geom.Pt) {
	h.core.Pointer(shell.Pointer{Kind: shell.PointerMove, Pos: p, Source: shell.SourceTouch})
}

// TestSelectionAreaTouchDragScrolls verifies that on touch, a plain drag (no
// long-press) does NOT select — it falls through to the scroll — so a
// text-heavy list stays scrollable by touch.
func TestSelectionAreaTouchDragScrolls(t *testing.T) {
	h, err := NewHeadless(listSelApp{items: []string{"alpha", "beta", "gamma", "delta"}},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	touchDown(h, geom.Pt{X: 1, Y: 6})  // on "alpha"
	touchMove(h, geom.Pt{X: 5, Y: 60}) // drag down immediately (no hold)
	h.Release(geom.Pt{X: 5, Y: 60})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "" {
		t.Fatalf("touch drag without long-press should scroll, not select; copied %q", got)
	}
}

// Copying nothing only proves the selection stayed out of it. The gesture has
// to actually reach the scroller: a handler that wins a drag on depth and then
// declines to act on it selects nothing *and* scrolls nothing, and the check
// above cannot tell that apart from working correctly.
func TestSelectionAreaTouchDragReachesTheScroller(t *testing.T) {
	ctrl := &widget.ScrollController{}
	h, err := NewHeadless(scrollSelApp{ctrl: ctrl},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	touchDown(h, geom.Pt{X: 20, Y: 100}) // on text
	touchMove(h, geom.Pt{X: 20, Y: 20})  // drag up immediately, no hold
	h.Release(geom.Pt{X: 20, Y: 20})
	h.Render()

	if ctrl.Offset() <= 0 {
		t.Errorf("offset %v: the touch drag selected nothing and scrolled nothing", ctrl.Offset())
	}
}

// scrollSelApp is listSelApp with a controller, so the scroll offset can be
// read back.
type scrollSelApp struct{ ctrl *widget.ScrollController }

func (a scrollSelApp) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 30)
	for i := range rows {
		rows[i] = widget.Sized{H: 20, Child: widget.Text{S: "alpha beta gamma", Size: 14}}
	}
	return widget.SelectionArea{Child: widget.Scroll{
		Controller: a.ctrl,
		Child:      widget.Column(rows...),
	}}
}

// TestSelectionAreaTouchLongPressSelects verifies the touch entry point: a
// long-press selects the word under it, and a following drag extends the
// selection.
func TestSelectionAreaTouchLongPressSelects(t *testing.T) {
	h, err := NewHeadless(listSelApp{items: []string{"alpha", "beta", "gamma", "delta"}},
		Config{Size: geom.Size{W: 400, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	touchDown(h, geom.Pt{X: 10, Y: 6}) // on "alpha"
	h.Step(longPressSeconds + 0.05)    // hold → long-press fires, selects the word
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "alpha" {
		t.Fatalf("long-press should select the word; copied %q", got)
	}
	// Now drag down to extend across items.
	touchMove(h, geom.Pt{X: 3000, Y: 60})
	h.Release(geom.Pt{X: 3000, Y: 60})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); !strings.Contains(got, "alpha") || !strings.Contains(got, "gamma") {
		t.Fatalf("long-press then drag should extend across items; copied %q", got)
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
