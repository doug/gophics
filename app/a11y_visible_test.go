package app

import (
	"image"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// headerListApp is a sticky header over a scrolling list of labeled rows:
// the layout in which rows scroll under something else and must not be
// offered to a screen reader as if they were still tappable there.
type headerListApp struct{ sc *widget.ScrollController }

func (a headerListApp) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 40)
	for i := range rows {
		rows[i] = widget.Semantics{Label: "row", Child: widget.Sized{W: 200, H: 40}}
	}
	return widget.Column(
		widget.Semantics{Label: "header", Child: widget.Sized{W: 200, H: 100}},
		widget.Expand(widget.Scroll{Controller: a.sc, Child: widget.Column(rows...)}),
	)
}

func a11yNode(nodes []A11yNode, id int) A11yNode {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	return A11yNode{ID: -1}
}

// The tree handed to the platform bridge carries each node's visible bounds,
// and leaves out nodes that are wholly clipped away. It used to publish every
// node's unclipped Rect, so with the list scrolled by 60 the first row (40–80,
// entirely under the 100px header) was a tappable element at those bounds,
// and explore-by-touch on the header returned it — it is smaller than the
// header it overlaps.
func TestA11yTreePublishesVisibleBounds(t *testing.T) {
	sc := &widget.ScrollController{}
	h, err := NewHeadless(headerListApp{sc}, Config{Size: geom.Size{W: 200, H: 300}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	sc.JumpTo(60)
	h.Render()

	nodes := h.core.A11yTree(1)
	var rows []A11yNode
	for _, n := range nodes {
		if n.Label == "row" {
			rows = append(rows, n)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no rows published")
	}
	// The viewport is 100..300 and shows rows 1 (its top 20px under the
	// header) through 6 (its bottom 20px under the fold); row 0 is entirely
	// under the header and rows 7+ entirely below the fold.
	if len(rows) != 6 {
		t.Errorf("published %d rows, want the 6 with a visible part", len(rows))
	}
	first := rows[0]
	if first.Y != 100 || first.H != 20 {
		t.Errorf("first published row spans y=%d h=%d, want the visible strip y=100 h=20", first.Y, first.H)
	}
	if last := rows[len(rows)-1]; last.Y+last.H > 300 {
		t.Errorf("last published row reaches y=%d, past the viewport's bottom", last.Y+last.H)
	}

	hit := a11yNode(nodes, h.core.A11yHitTest(10, 80, 1))
	if hit.Label != "header" {
		t.Errorf("explore-by-touch on the header returned %q at y=%d h=%d", hit.Label, hit.Y, hit.H)
	}
	hit = a11yNode(nodes, h.core.A11yHitTest(10, 110, 1))
	if hit.Label != "row" || hit.ID != first.ID {
		t.Errorf("explore-by-touch just under the header returned %q (id %d), want the half-hidden row %d", hit.Label, hit.ID, first.ID)
	}
	if h.core.A11yHitTest(10, 299, 1) < 0 {
		t.Error("explore-by-touch at the viewport's last pixel found nothing")
	}
}

// semLabelApp is a labeled group whose label can change without any pixel
// changing: the Child paints nothing.
type semLabelApp struct{ label *string }

func (a semLabelApp) Build(widget.Ctx) widget.Widget {
	return widget.Semantics{Label: *a.label, Child: widget.Sized{W: 50, H: 50}}
}

func groupLabel(nodes []shell.A11yNode) string {
	for _, n := range nodes {
		if n.Role == "group" && n.ParentID != -1 {
			return n.Label
		}
	}
	return ""
}

// A rebuild that changes semantics but paints nothing different must still
// reach the screen reader. Publishing was gated on the renderer's "scene
// changed" signal alone, so a label, hint or checked state that changed
// without a visual difference was never republished.
func TestA11yRepublishedOnSemanticOnlyChange(t *testing.T) {
	label := "one"
	h, err := NewHandler(semLabelApp{&label}, Config{Size: geom.Size{W: 100, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	sh := h.(*shellHandler)
	at := &fakeAT{}
	w := fakeA11yWindow{at: at}
	f := &fakeFrame{size: geom.Size{W: 100, H: 100}, scale: 1,
		tgt: shell.PixelTarget{Put: func(*image.RGBA, geom.Rect) {}}}
	sh.Frame(w, f, 0)
	sh.Frame(w, f, 1.0/60)
	if len(at.trees) != 1 || groupLabel(at.trees[0]) != "one" {
		t.Fatalf("after two frames: %d trees, label %q", len(at.trees), groupLabel(at.trees[len(at.trees)-1]))
	}

	label = "two"
	sh.core.Owner.RebuildAll()
	sh.Frame(w, f, 1.0/60)
	if got := groupLabel(at.trees[len(at.trees)-1]); got != "two" {
		t.Errorf("label changed to %q with no visual change; the bridge still has %q", label, got)
	}
	if len(at.trees) != 2 {
		t.Errorf("published %d trees, want 2", len(at.trees))
	}

	// A rebuild that changes nothing is diffed away rather than republished.
	sh.core.Owner.RebuildAll()
	sh.Frame(w, f, 1.0/60)
	if len(at.trees) != 2 {
		t.Errorf("an unchanged rebuild republished the tree (%d trees)", len(at.trees))
	}
}
