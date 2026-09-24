package layout

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// semLeaf is a fixed-size box that contributes one semantic node.
type semLeaf struct {
	size geom.Size
	info SemInfo
}

func (b *semLeaf) Layout(Constraints) geom.Size { return b.size }
func (b *semLeaf) Size() geom.Size              { return b.size }
func (b *semLeaf) Paint(paint.Canvas, geom.Pt)  {}
func (b *semLeaf) AddHits(geom.Pt, *[]Hit)      {}
func (b *semLeaf) Semantics() SemInfo           { return b.info }

// semChild is a child placed at an offset inside a semContainer.
type semChild struct {
	box Box
	at  geom.Pt
}

// semContainer is a fixed-size box with positioned children. With clips set
// it is a viewport: it implements Clipper the way a scroll viewport does.
type semContainer struct {
	size  geom.Size
	kids  []semChild
	clips bool
	info  SemInfo
}

func (b *semContainer) Layout(Constraints) geom.Size { return b.size }
func (b *semContainer) Size() geom.Size              { return b.size }
func (b *semContainer) Paint(paint.Canvas, geom.Pt)  {}
func (b *semContainer) AddHits(geom.Pt, *[]Hit)      {}
func (b *semContainer) ClipsChildren() bool          { return b.clips }
func (b *semContainer) Semantics() SemInfo           { return b.info }
func (b *semContainer) VisitChildren(visit func(Box, geom.Pt)) {
	for _, k := range b.kids {
		visit(k.box, k.at)
	}
}

func text(label string, w, h float32) *semLeaf {
	return &semLeaf{size: geom.Size{W: w, H: h}, info: SemInfo{Role: RoleText, Label: label}}
}

func byLabel(t *testing.T, nodes []SemNode, label string) SemNode {
	t.Helper()
	for _, n := range FlattenSemantics(nodes) {
		if n.Label == label {
			return n
		}
	}
	t.Fatalf("no node labeled %q in %v", label, semLabels(nodes))
	return SemNode{}
}

func semLabels(nodes []SemNode) []string {
	var out []string
	for _, n := range FlattenSemantics(nodes) {
		out = append(out, n.Role.String()+":"+n.Label)
	}
	return out
}

// A viewport narrows what its descendants can be tapped at. Rect stays the
// laid-out rect for every row; Visible is the strip on screen, and Offscreen
// says the strip is empty. The fold cases are the ones that regress silently:
// a hairline separator exactly on the viewport's bottom edge is still on
// screen, while a row that merely touches that edge is not.
func TestSemanticsClipperNarrowsVisible(t *testing.T) {
	viewport := &semContainer{size: geom.Size{W: 100, H: 100}, clips: true, kids: []semChild{
		{text("in", 100, 40), geom.Pt{X: 0, Y: 0}},
		{text("half", 100, 40), geom.Pt{X: 0, Y: 80}},
		{text("out", 100, 40), geom.Pt{X: 0, Y: 150}},
		{text("touching", 100, 40), geom.Pt{X: 0, Y: 100}},
		{text("fold", 100, 0), geom.Pt{X: 0, Y: 100}},
		{text("below", 100, 0), geom.Pt{X: 0, Y: 110}},
	}}
	root := &semContainer{size: geom.Size{W: 300, H: 300}, kids: []semChild{
		{viewport, geom.Pt{X: 10, Y: 20}},
	}}
	root.Layout(Unbounded())
	nodes := CollectSemantics(root)

	in := byLabel(t, nodes, "in")
	if in.Offscreen || in.Visible != in.Rect {
		t.Errorf("row in full view: Visible %v, Rect %v, Offscreen %v", in.Visible, in.Rect, in.Offscreen)
	}
	half := byLabel(t, nodes, "half")
	if half.Offscreen {
		t.Error("row half under the fold reported Offscreen")
	}
	if want := (geom.Rect{Min: geom.Pt{X: 10, Y: 100}, Max: geom.Pt{X: 110, Y: 120}}); half.Visible != want {
		t.Errorf("half-hidden row Visible = %v, want %v", half.Visible, want)
	}
	if want := (geom.Rect{Min: geom.Pt{X: 10, Y: 100}, Max: geom.Pt{X: 110, Y: 140}}); half.Rect != want {
		t.Errorf("half-hidden row Rect = %v, want the unclipped %v", half.Rect, want)
	}
	for _, label := range []string{"out", "touching", "below"} {
		n := byLabel(t, nodes, label)
		if !n.Offscreen || n.Visible != (geom.Rect{}) {
			t.Errorf("%q: Offscreen %v, Visible %v; want offscreen with the zero Visible", label, n.Offscreen, n.Visible)
		}
	}
	fold := byLabel(t, nodes, "fold")
	if fold.Offscreen {
		t.Error("a hairline on the viewport's edge is on screen; it was reported Offscreen")
	}
	if want := (geom.Rect{Min: geom.Pt{X: 10, Y: 120}, Max: geom.Pt{X: 110, Y: 120}}); fold.Visible != want {
		t.Errorf("hairline Visible = %v, want it kept at its coordinate, %v", fold.Visible, want)
	}
}

// A viewport scrolled wholly out of the window closes the clip to nothing.
// Intersect reports that as the zero Rect, which is also the rect of a
// hairline at the origin — a child there must still come out Offscreen.
func TestSemanticsClipClosedIsNotAHairline(t *testing.T) {
	viewport := &semContainer{size: geom.Size{W: 100, H: 100}, clips: true, kids: []semChild{
		{text("origin", 0, 0), geom.Pt{}},
		{text("row", 100, 40), geom.Pt{}},
	}}
	root := &semContainer{size: geom.Size{W: 300, H: 300}, kids: []semChild{
		{viewport, geom.Pt{X: 0, Y: -500}},
	}}
	nodes := CollectSemantics(root)
	for _, label := range []string{"origin", "row"} {
		if n := byLabel(t, nodes, label); !n.Offscreen {
			t.Errorf("%q inside a viewport scrolled off the window is not Offscreen (Visible %v)", label, n.Visible)
		}
	}
}

// The root's own rect is the outermost clip, without implementing Clipper.
func TestSemanticsRootClips(t *testing.T) {
	root := &semContainer{size: geom.Size{W: 100, H: 100}, kids: []semChild{
		{text("page", 100, 100), geom.Pt{X: 100, Y: 0}},
		{text("edge", 100, 100), geom.Pt{X: 50, Y: 0}},
	}}
	nodes := CollectSemantics(root)
	if n := byLabel(t, nodes, "page"); !n.Offscreen {
		t.Errorf("a page slid past the root's edge is not Offscreen: %+v", n)
	}
	if n := byLabel(t, nodes, "edge"); n.Offscreen || n.Visible.Dx() != 50 {
		t.Errorf("a page half past the root's edge: Offscreen %v, Visible %v", n.Offscreen, n.Visible)
	}
}

// Nested clippers narrow cumulatively: a row inside an inner viewport is
// bounded by the outer one too.
func TestSemanticsNestedClippers(t *testing.T) {
	inner := &semContainer{size: geom.Size{W: 100, H: 200}, clips: true, kids: []semChild{
		{text("row", 100, 40), geom.Pt{X: 0, Y: 80}},
	}}
	outer := &semContainer{size: geom.Size{W: 100, H: 100}, clips: true, kids: []semChild{
		{inner, geom.Pt{}},
	}}
	root := &semContainer{size: geom.Size{W: 300, H: 300}, kids: []semChild{{outer, geom.Pt{}}}}
	n := byLabel(t, CollectSemantics(root), "row")
	if n.Offscreen || n.Visible.Dy() != 20 {
		t.Errorf("row under the outer fold: Offscreen %v, Visible %v; want a 20px strip", n.Offscreen, n.Visible)
	}
}

// A container with a label of its own keeps the text inside it: "Settings"
// is the group's name, not a replacement for the lines it contains. Text
// used to be dropped here — neither joined into the label (that only happens
// when the label is empty) nor kept as a child — so every plain line inside
// a labeled group was unreadable to assistive technology.
func TestLabeledContainerKeepsTextChildren(t *testing.T) {
	group := &semContainer{size: geom.Size{W: 200, H: 100}, info: SemInfo{Role: RoleGroup, Label: "Settings"}, kids: []semChild{
		{text("Version 1.2", 200, 20), geom.Pt{}},
		{&semContainer{size: geom.Size{W: 200, H: 20}, info: SemInfo{Role: RoleButton, OnActivate: func() {}},
			kids: []semChild{{text("Reset", 200, 20), geom.Pt{}}}}, geom.Pt{Y: 20}},
	}}
	nodes := CollectSemantics(group)
	if len(nodes) != 1 || nodes[0].Label != "Settings" {
		t.Fatalf("root = %v", semLabels(nodes))
	}
	if got := semLabels(nodes[0].Children); len(got) != 2 || got[0] != "text:Version 1.2" || got[1] != "button:Reset" {
		t.Errorf("children of the labeled group = %v, want the text and the button", got)
	}
}

// A control names itself from its text: "Send" is the button, not a text
// inside a button. That absorption is unchanged, whether the label came from
// the text or was given explicitly and the text merely repeats it.
func TestLabeledControlAbsorbsText(t *testing.T) {
	for _, label := range []string{"", "Send"} {
		btn := &semContainer{size: geom.Size{W: 100, H: 40}, info: SemInfo{Role: RoleButton, Label: label},
			kids: []semChild{{text("Send", 100, 40), geom.Pt{}}}}
		nodes := CollectSemantics(btn)
		if len(nodes) != 1 || nodes[0].Label != "Send" {
			t.Fatalf("label %q: nodes = %v", label, semLabels(nodes))
		}
		if len(nodes[0].Children) != 0 {
			t.Errorf("label %q: button kept %v as children; its text is its name", label, semLabels(nodes[0].Children))
		}
	}
}
