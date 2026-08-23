package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

func a11yTree() []widget.TreeNode {
	return []widget.TreeNode{
		{ID: "src", Child: widget.Text{S: "src"}, Children: []widget.TreeNode{
			{ID: "main", Child: widget.Text{S: "main.go"}},
		}},
		{ID: "readme", Child: widget.Text{S: "README"}},
	}
}

// treeItems renders a tree and returns the treeitem nodes an assistive
// technology would receive.
//
// Rendered rather than constructed: the point is that the state survives the
// semantics walk, which is what every platform bridge consumes. A field set on
// the widget and dropped before that walk reaches nobody.
func treeItems(t *testing.T, w widget.Tree) []layout.SemNode {
	t.Helper()
	a := apptest.New(t, w, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 800}, Font: goregular.TTF,
	}))
	a.Render()

	var items []layout.SemNode
	for _, n := range a.Nodes() {
		if n.Role == layout.RoleTreeItem {
			items = append(items, n)
		}
	}
	return items
}

// Expanded has to reach the collected semantics, and only branches may carry
// it: "collapsed" and "has nothing to open" are different announcements, and a
// tree that calls every leaf collapsed invites opening rows that never open.
func TestTreeExpandedReachesTheSemanticsTree(t *testing.T) {
	items := treeItems(t, widget.Tree{
		Nodes:             a11yTree(),
		InitiallyExpanded: []string{"src"},
	})
	if len(items) == 0 {
		t.Fatal("no treeitem nodes were collected; the tree is invisible to " +
			"assistive technology and this test would otherwise prove nothing")
	}

	branches, leaves, open := 0, 0, 0
	for _, n := range items {
		if n.Expanded == nil {
			leaves++
			continue
		}
		branches++
		if *n.Expanded {
			open++
		}
	}
	if branches == 0 {
		t.Error("no collected row carries expanded state, so nothing announces " +
			"which rows can be opened")
	}
	if leaves == 0 {
		t.Error("every collected row claims to be expandable; a leaf must carry " +
			"no state at all")
	}
	if open == 0 {
		t.Error("the tree was built with src expanded, but no collected row " +
			"reports itself open")
	}
}

// A collapsed branch must say so rather than say nothing.
func TestTreeCollapsedBranchReportsFalse(t *testing.T) {
	items := treeItems(t, widget.Tree{Nodes: a11yTree()})
	for _, n := range items {
		if n.Expanded != nil && *n.Expanded {
			t.Error("nothing was expanded, yet a row reports itself open")
		}
	}
	found := false
	for _, n := range items {
		if n.Expanded != nil {
			found = true
		}
	}
	if !found {
		t.Error("a collapsed tree published no expandable rows at all")
	}
}

// The tree itself must be announced as a tree, or its rows are treeitems
// belonging to nothing.
func TestTreeRoleReachesTheSemanticsTree(t *testing.T) {
	a := apptest.New(t, widget.Tree{Nodes: a11yTree()}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 800}, Font: goregular.TTF,
	}))
	a.Render()
	for _, n := range a.Nodes() {
		if n.Role == layout.RoleTree {
			return
		}
	}
	t.Error("no node with the tree role was collected")
}

// The disclosure glyph must not become the row's label. Semantics absorbs the
// text of a node's subtree when the node has no label of its own, so an
// unhidden mark makes a row read "▸ src" — announcing a triangle as content.
func TestTreeRowLabelExcludesTheChevron(t *testing.T) {
	items := treeItems(t, widget.Tree{Nodes: a11yTree()})
	if len(items) == 0 {
		t.Fatal("no treeitem nodes collected")
	}
	for _, n := range items {
		for _, mark := range []string{"▸", "▾"} {
			if containsRune(n.Label, mark) {
				t.Errorf("row label %q contains the disclosure glyph %q; it is "+
					"decoration and the row already publishes its state", n.Label, mark)
			}
		}
	}
}

// Role names cross to every platform as ARIA strings, so a role with no name
// reaches the bridges as an empty attribute.
func TestTreeRoleNames(t *testing.T) {
	if got := layout.RoleTree.String(); got != "tree" {
		t.Errorf("RoleTree.String() = %q, want %q", got, "tree")
	}
	if got := layout.RoleTreeItem.String(); got != "treeitem" {
		t.Errorf("RoleTreeItem.String() = %q, want %q", got, "treeitem")
	}
}

func containsRune(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Rows share a left edge, and depth is the only thing that moves content right.
//
// A plain Column centres its children, which is what this widget used to do:
// rows of different widths each sat in the middle of the tree, so the
// indentation step no longer measured from anything and the hierarchy stopped
// reading as one. It renders as a centred list, and every existing test still
// passed — the shape is only visible in the geometry.
func TestTreeRowsShareALeftEdge(t *testing.T) {
	a := apptest.New(t, widget.Tree{
		Nodes:             a11yTree(),
		InitiallyExpanded: []string{"src"},
	}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 800}, Font: goregular.TTF,
	}))
	a.Render()

	var lefts []float32
	for _, n := range a.Nodes() {
		if n.Role == layout.RoleTreeItem {
			lefts = append(lefts, n.Rect.Min.X)
		}
	}
	if len(lefts) < 3 {
		t.Fatalf("only %d rows laid out; this test needs the expanded tree to "+
			"check anything", len(lefts))
	}
	for i, x := range lefts {
		if x != lefts[0] {
			t.Errorf("row %d starts at x=%g but row 0 starts at x=%g — the rows "+
				"are not sharing a left edge, so indentation measures from "+
				"nothing and the hierarchy does not read", i, x, lefts[0])
		}
	}
}
