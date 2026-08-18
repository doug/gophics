package widget

import "testing"

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
