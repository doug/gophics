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
		widget.Text{Value: s.a, Size: 14},
		widget.Text{Value: s.b, Size: 14},
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
		Build:           func(i int) widget.Widget { return widget.Text{Value: s.items[i], Size: 14} },
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
		Child: widget.Text{Value: s.text, Size: 14},
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
		rows[i] = widget.Sized{H: 20, Child: widget.Text{Value: "alpha beta gamma", Size: 14}}
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
	touchDown(h, geom.Pt{X: 10, Y: 6})                        // on "alpha"
	h.Step(shell.GestureTuning{}.Resolved().LongPress + 0.05) // hold → long-press fires, selects the word
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

// Native selection behaviours — the gestures a reader brings from the
// platform's own text views. Each is driven through the real dispatcher, so a
// press that must count as the second click really arrives as one.

// doubleClick presses twice at p inside the double-tap window, leaving the
// button down so a drag can follow, as a double-click-drag does.
func pressNth(h *Headless, p geom.Pt, n int) {
	for i := 0; i < n; i++ {
		h.Press(p)
		if i < n-1 {
			h.Release(p)
		}
		h.Render()
	}
}

// A double click takes the word under it, not a caret, and a drag from there
// runs by whole words: no half-word is ever left at either end.
func TestSelectionAreaDoubleClickSelectsWordAndDragsByWord(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	pressNth(h, geom.Pt{X: 10, Y: 6}, 2) // inside "alpha"
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "alpha" {
		t.Fatalf("double click copied %q, want \"alpha\"", got)
	}
	// Drag into the middle of "gamma": the whole word comes with it.
	h.Move(geom.Pt{X: 92, Y: 6})
	h.Release(geom.Pt{X: 92, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); !strings.HasPrefix(got, "alpha beta") || !strings.HasSuffix(got, "gamma") {
		t.Fatalf("word drag copied %q, want it to start at \"alpha beta\" and end on a whole \"gamma\"", got)
	}
}

// A third click takes the paragraph — the fragment the pointer is in, which is
// what a browser's triple click takes.
func TestSelectionAreaTripleClickSelectsParagraph(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	pressNth(h, geom.Pt{X: 10, Y: 6}, 3)
	h.Release(geom.Pt{X: 10, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "alpha beta gamma" {
		t.Fatalf("triple click copied %q, want the whole first line", got)
	}
}

// Shift-click keeps the anchor and moves the far end, instead of starting over.
func TestSelectionAreaShiftClickExtends(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	h.Press(geom.Pt{X: 1, Y: 6})
	h.Release(geom.Pt{X: 1, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyShift, shell.ModShift) // shift down for the next press
	h.Press(geom.Pt{X: 40, Y: 6})
	h.Release(geom.Pt{X: 40, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got == "" || !strings.HasPrefix("alpha beta gamma", got) {
		t.Fatalf("shift-click copied %q, want a prefix of the first line", got)
	}
}

// Cmd/Ctrl+A takes every fragment in the area, which is what the keystroke
// means everywhere else.
func TestSelectionAreaSelectAll(t *testing.T) {
	h := selAreaHarness(t, "Hello", "World")
	h.Press(geom.Pt{X: 1, Y: 6}) // focus the area
	h.Release(geom.Pt{X: 1, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyA, shell.ModSuper)
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got, want := clip(h), "Hello\nWorld"; got != want {
		t.Fatalf("select all copied %q, want %q", got, want)
	}
}

// Right-click with nothing selected takes the word under the pointer and
// offers the menu, as both desktops do.
func TestSelectionAreaRightClickSelectsWordAndOffersCopy(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 10, Y: 6}, Button: 1})
	h.Render()
	var labels []string
	for _, n := range h.Semantics() {
		labels = append(labels, n.Label)
	}
	if !strings.Contains(strings.Join(labels, "|"), "Copy") {
		t.Fatalf("no edit menu after a right click; semantics: %v", labels)
	}
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "alpha" {
		t.Fatalf("right click selected %q, want the word under it", got)
	}
}

// A finger's long-press selection gets grips, and dragging one adjusts the
// selection rather than replacing it — the only way to fix a selection on a
// phone, where there is no shift-arrow.
func TestSelectionAreaTouchHandleAdjustsSelection(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	h.TouchPress(geom.Pt{X: 10, Y: 6}) // inside "alpha"
	h.Step(shell.GestureTuning{}.Resolved().LongPress + 0.05)
	h.Render()
	h.TouchRelease(geom.Pt{X: 10, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); got != "alpha" {
		t.Fatalf("long press copied %q, want \"alpha\"", got)
	}
	grip, ok := gripBelowFirstLine(h)
	if !ok {
		t.Fatal("no selection grip drawn after a finger made the selection")
	}
	// Grab the right-hand grip and pull it across "beta".
	h.TouchPress(grip)
	h.TouchMove(geom.Pt{X: 72, Y: 6})
	h.TouchRelease(geom.Pt{X: 72, Y: 6})
	h.Render()
	h.KeyMod(shell.KeyC, shell.ModSuper)
	if got := clip(h); !strings.HasPrefix(got, "alpha") || got == "alpha" {
		t.Fatalf("dragging the grip gave %q, want the selection grown past \"alpha\" from the same start", got)
	}
}

// A mouse selection gets no grips: a desktop reader adjusts with shift-click,
// and two dots under the text would only puzzle them.
func TestSelectionAreaMouseSelectionHasNoHandles(t *testing.T) {
	h := selAreaHarness(t, "alpha beta gamma", "delta epsilon")
	h.DragTo(geom.Pt{X: 1, Y: 6}, geom.Pt{X: 72, Y: 6})
	h.Release(geom.Pt{X: 72, Y: 6})
	h.Render()
	if _, ok := gripBelowFirstLine(h); ok {
		t.Fatal("a mouse selection drew a selection grip")
	}
}

// gripBelowFirstLine finds a drawn grip by its own pixels: the dot is the
// selection colour at full alpha, which nothing else here paints — the
// highlight band is the same hue at a third of the alpha, and over the pale
// background that never reaches full saturation. The rightmost dot is the far
// end of the selection; its centre is what a finger would aim at.
func gripBelowFirstLine(h *Headless) (geom.Pt, bool) {
	img := h.Render()
	b := img.Bounds()
	var xs, ys []float32
	var maxX float32
	for y := b.Min.Y; y < min(b.Min.Y+30, b.Max.Y); y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if bl > 0xe000 && bl > r+0x6000 && g > r {
				xs, ys = append(xs, float32(x)), append(ys, float32(y))
				maxX = max(maxX, float32(x))
			}
		}
	}
	if len(xs) == 0 {
		return geom.Pt{}, false
	}
	// Average the rightmost dot alone: anything within one dot width of the
	// furthest pixel belongs to it.
	var sx, sy, n float32
	for i := range xs {
		if xs[i] > maxX-2*selGripRadius {
			sx, sy, n = sx+xs[i], sy+ys[i], n+1
		}
	}
	return geom.Pt{X: sx / n, Y: sy / n}, true
}

// selGripRadius mirrors the widget's own handle radius; the test only needs it
// to tell one dot from the other.
const selGripRadius = 7

// Dragging a selection toward the bottom of a scrolling page scrolls it, the
// way every platform does — otherwise the selection stops at whatever happened
// to be on screen when the drag began.
func TestSelectionAreaDragScrollsThePageIntoView(t *testing.T) {
	items := make([]string, 60)
	for i := range items {
		items[i] = "line " + string(rune('a'+i%26))
	}
	h, err := NewHeadless(listSelApp{items: items},
		Config{Size: geom.Size{W: 300, H: 100}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Start on the first line and drag to the bottom edge, holding there.
	h.Press(geom.Pt{X: 4, Y: 6})
	h.Move(geom.Pt{X: 120, Y: 96})
	h.Render()
	first := len(clipAfterCopy(h))
	for i := 0; i < 8; i++ {
		h.Move(geom.Pt{X: 120, Y: 96})
		h.Render()
	}
	grown := len(clipAfterCopy(h))
	h.Release(geom.Pt{X: 120, Y: 96})
	if grown <= first {
		t.Fatalf("holding the drag at the bottom edge selected no further lines: %d then %d", first, grown)
	}
}

// clipAfterCopy copies the current selection and returns it.
func clipAfterCopy(h *Headless) string {
	h.Clipboard().S = ""
	h.KeyMod(shell.KeyC, shell.ModSuper)
	return h.Clipboard().S
}
