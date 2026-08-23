package widget

import (
	"testing"

	"github.com/doug/gophics/layout"
)

func sampleTree() []TreeNode {
	return []TreeNode{
		{ID: "src", Child: Text{S: "src"}, Children: []TreeNode{
			{ID: "main", Child: Text{S: "main.go"}},
			{ID: "lib", Child: Text{S: "lib"}, Children: []TreeNode{
				{ID: "deep", Child: Text{S: "deep.go"}},
			}},
		}},
		{ID: "readme", Child: Text{S: "README"}},
	}
}

// rowsFor builds the tree's rows without a live element tree.
func rowsFor(t *testing.T, s *treeState) []Widget {
	t.Helper()
	var rows []Widget
	s.appendRows(&rows, s.W().Nodes, 0, 16)
	return rows
}

func newTreeState(t *testing.T, w Tree, open ...string) *treeState {
	t.Helper()
	s := &treeState{}
	s.setWidget(w)
	s.init = true
	s.open = map[string]bool{}
	for _, id := range open {
		s.open[id] = true
	}
	return s
}

// A collapsed tree shows only its roots: children of a closed node must not be
// built, or a deep hierarchy costs its whole size on every frame.
func TestTreeCollapsedShowsRootsOnly(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	if got := len(rowsFor(t, s)); got != 2 {
		t.Errorf("collapsed tree built %d rows, want 2", got)
	}
}

func TestTreeExpandRevealsChildren(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()}, "src")
	if got := len(rowsFor(t, s)); got != 4 { // src, main.go, lib, README
		t.Errorf("one level open built %d rows, want 4", got)
	}
	s2 := newTreeState(t, Tree{Nodes: sampleTree()}, "src", "lib")
	if got := len(rowsFor(t, s2)); got != 5 {
		t.Errorf("two levels open built %d rows, want 5", got)
	}
}

// A nested node stays folded when its parent is closed, however it was left.
func TestTreeHiddenSubtreeStaysHidden(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()}, "lib") // parent "src" closed
	if got := len(rowsFor(t, s)); got != 2 {
		t.Errorf("built %d rows, want 2 — an open node under a closed parent is not visible", got)
	}
}

func TestTreeToggleFlips(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	if s.isOpen("src") {
		t.Fatal("fixture should start closed")
	}
	open, store := s.toggleDecision("src")
	if !open || !store {
		t.Fatalf("first toggle = (%v, %v), want (true, true)", open, store)
	}
	s.open["src"] = open

	open, _ = s.toggleDecision("src")
	if open {
		t.Error("second toggle should close the node")
	}
}

// When the app supplies Expanded it owns the set, and the tree must report
// rather than write — otherwise the app's state and the tree's disagree.
func TestTreeControlledModeDoesNotWrite(t *testing.T) {
	ctrl := map[string]bool{"src": true}
	var gotID string
	var gotOpen bool
	s := newTreeState(t, Tree{
		Nodes:    sampleTree(),
		Expanded: ctrl,
		OnToggle: func(id string, open bool) { gotID, gotOpen = id, open },
	})

	if !s.isOpen("src") {
		t.Error("controlled mode should read the app's map")
	}
	open, store := s.toggleDecision("src")
	if store {
		t.Error("the tree would write into a map it does not own")
	}
	if open {
		t.Error("toggling an open node should close it")
	}
	// The notification still has to reach the app, which is the only way it
	// learns to update its own map.
	if f := s.W().OnToggle; f != nil {
		f("src", open)
	}
	if ctrl["src"] != true {
		t.Error("the app's map changed without the app doing it")
	}
	if gotID != "src" || gotOpen != false {
		t.Errorf("OnToggle got (%q, %v), want (src, false)", gotID, gotOpen)
	}
}

// InitiallyExpanded is a starting state, not a control, so a later toggle wins.
func TestTreeInitiallyExpanded(t *testing.T) {
	s := &treeState{}
	s.setWidget(Tree{Nodes: sampleTree(), InitiallyExpanded: []string{"src"}})
	s.Build(Ctx{})
	if !s.isOpen("src") {
		t.Fatal("InitiallyExpanded did not open the node")
	}
	// It is a starting state, not a control: a later toggle wins.
	open, store := s.toggleDecision("src")
	if open || !store {
		t.Errorf("toggle after initial expansion = (%v, %v), want (false, true)", open, store)
	}
}

// Leaves reserve the affordance's width so their content lines up with
// expandable siblings instead of shifting left.
func TestTreeLeafReservesAffordanceWidth(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	leaf := s.disclosureFor("readme", false, false)
	sz, ok := leaf.(Sized)
	if !ok {
		t.Fatalf("leaf affordance is %T, want a Sized spacer", leaf)
	}
	if sz.W != 16 || sz.Child != nil {
		t.Errorf("leaf affordance = %+v, want a blank 16pt spacer", sz)
	}
}

// semOf returns the Semantics wrapper around row i, which is what assistive
// technology reads.
func semOf(t *testing.T, rows []Widget, i int) Semantics {
	t.Helper()
	if i >= len(rows) {
		t.Fatalf("row %d does not exist (have %d)", i, len(rows))
	}
	sem, ok := rows[i].(Semantics)
	if !ok {
		t.Fatalf("row %d is %T, not a Semantics node — the row is invisible to "+
			"a screen reader", i, rows[i])
	}
	return sem
}

// A tree has to say it is a tree, and its rows have to say they are rows in
// one. Without this the whole widget reaches assistive technology as anonymous
// columns and rows: navigable, but with no structure to navigate by, and no way
// to know a row can be opened at all.
func TestTreeRowsAnnounceThemselvesAsTreeItems(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	rows := rowsFor(t, s)
	if len(rows) == 0 {
		t.Fatal("no rows built; this test would pass without checking anything")
	}
	for i := range rows {
		if got := semOf(t, rows, i).Role; got != roleTreeItem() {
			t.Errorf("row %d has role %v, want treeitem", i, got)
		}
	}
}

// Expanded is the state a screen reader announces and the state that tells the
// user a row is worth opening. A branch reports true or false; a leaf reports
// nothing at all, because "collapsed" would invite opening a row that never
// opens.
func TestTreeAnnouncesExpandedOnlyForBranches(t *testing.T) {
	// Collapsed: src is a branch, readme is a leaf.
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	rows := rowsFor(t, s)

	src := semOf(t, rows, 0)
	if src.Expanded == nil {
		t.Fatal("a branch reported no expanded state, so nothing tells the user " +
			"the row can be opened")
	}
	if *src.Expanded {
		t.Error("a collapsed branch reported itself expanded")
	}

	leaf := semOf(t, rows, 1)
	if leaf.Expanded != nil {
		t.Errorf("a leaf reported expanded=%v; a leaf has no state to be in",
			*leaf.Expanded)
	}

	// Expanded: the same branch now reports true.
	s = newTreeState(t, Tree{Nodes: sampleTree()}, "src")
	rows = rowsFor(t, s)
	if e := semOf(t, rows, 0).Expanded; e == nil || !*e {
		t.Error("an open branch did not report itself expanded")
	}
}

// treeStateOf digs the mounted treeState out of an element tree.
func treeStateOf(el *element) *treeState {
	if el == nil {
		return nil
	}
	if s, ok := el.state.(*treeState); ok {
		return s
	}
	if s := treeStateOf(el.child); s != nil {
		return s
	}
	for _, k := range el.kids {
		if s := treeStateOf(k); s != nil {
			return s
		}
	}
	return nil
}

// A row must be operable by assistive technology, not only by tapping the
// chevron. The chevron is a 16pt target the user cannot see; VoiceOver and
// TalkBack activate the row.
//
// Mounted rather than constructed, because activation ends in SetState and
// that needs a live element — the same reason toggleDecision exists.
func TestTreeRowsAreActivatable(t *testing.T) {
	o := newOwner()
	o.SetRoot(Tree{Nodes: sampleTree()})
	s := treeStateOf(o.root)
	if s == nil {
		t.Fatal("the tree did not mount")
	}

	rows := rowsFor(t, s)
	branch := semOf(t, rows, 0)
	if branch.OnActivate == nil {
		t.Fatal("a branch row has no activation, so a screen reader user cannot " +
			"open it — the only handler is on the chevron")
	}
	branch.OnActivate()
	if !s.isOpen("src") {
		t.Error("activating a branch row did not open it")
	}

	if semOf(t, rows, 1).OnActivate != nil {
		t.Error("a leaf row offers an activation that does nothing")
	}
}

// The disclosure glyph must not reach the label. Semantics absorbs the text of
// a node's subtree when the node has no label of its own, so an unhidden mark
// makes the row read "▸ src" — a triangle announced as content, and the real
// state left unsaid.
func TestTreeDisclosureIsHiddenFromSemantics(t *testing.T) {
	s := newTreeState(t, Tree{Nodes: sampleTree()})
	d := s.disclosureFor("src", true, false)

	sized, ok := d.(Sized)
	if !ok {
		t.Fatalf("disclosure is %T, want Sized", d)
	}
	inter, ok := sized.Child.(Interactive)
	if !ok {
		t.Fatalf("disclosure child is %T, want Interactive", sized.Child)
	}
	sem, ok := inter.Child.(Semantics)
	if !ok {
		t.Fatalf("the glyph is %T, not wrapped in Semantics — it will be read "+
			"aloud as part of the row's label", inter.Child)
	}
	if !sem.Hidden {
		t.Error("the disclosure glyph is exposed to assistive technology")
	}
}

// roleTreeItem keeps the layout import honest about what the rows claim to be.
func roleTreeItem() layout.Role { return layout.RoleTreeItem }
