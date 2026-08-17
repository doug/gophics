//go:build linux

package platform

import "testing"

// hasState reports whether the two-word AT-SPI bitset has a state set.
func hasState(bits []uint32, state uint) bool {
	i := state / 32
	return int(i) < len(bits) && bits[i]&(1<<(state%32)) != 0
}

// The states that make a node exist at all: a screen reader filters out
// anything without SHOWING and VISIBLE, so a tree missing them reads as empty
// even though every node is present and correct.
func TestStatesAlwaysShowing(t *testing.T) {
	bits := atspiStates(A11yNode{ID: 1, Role: "button"})
	if !hasState(bits, stateShowing) || !hasState(bits, stateVisible) {
		t.Error("a published node must be SHOWING and VISIBLE")
	}
}

// Disabled is expressed by clearing SENSITIVE and ENABLED, not by adding a
// state — get this backwards and a greyed-out button reads as available.
func TestDisabledClearsSensitive(t *testing.T) {
	on := atspiStates(A11yNode{ID: 1, Role: "button"})
	if !hasState(on, stateEnabled) || !hasState(on, stateSensitive) {
		t.Fatal("an enabled node should be ENABLED and SENSITIVE")
	}
	off := atspiStates(A11yNode{ID: 1, Role: "button", Disabled: true})
	if hasState(off, stateEnabled) || hasState(off, stateSensitive) {
		t.Error("a disabled node must be neither ENABLED nor SENSITIVE")
	}
}

// CHECKABLE is 41, which lands in the second word of the bitset. That is the
// one place the two-word encoding can silently go wrong.
func TestCheckableCrossesWordBoundary(t *testing.T) {
	bits := atspiStates(A11yNode{ID: 1, Role: "checkbox", Checkable: true, Checked: true})
	if len(bits) != 2 {
		t.Fatalf("state set has %d words, want 2", len(bits))
	}
	if !hasState(bits, stateCheckable) {
		t.Error("CHECKABLE (41) not set — check the second word")
	}
	if !hasState(bits, stateChecked) {
		t.Error("CHECKED (4) not set")
	}
	if bits[1] == 0 {
		t.Error("second word is empty, so state 41 cannot be set")
	}
}

// A checkbox that is checkable but unchecked must not report CHECKED.
func TestUncheckedIsNotChecked(t *testing.T) {
	bits := atspiStates(A11yNode{ID: 1, Role: "checkbox", Checkable: true})
	if hasState(bits, stateChecked) {
		t.Error("an unchecked checkbox reported CHECKED")
	}
	if !hasState(bits, stateCheckable) {
		t.Error("CHECKABLE should still be set")
	}
}

func TestRoleMapping(t *testing.T) {
	for aria, want := range map[string]uint32{
		"button":   rolePushButton,
		"checkbox": roleCheckBox,
		"link":     roleLink,
		"textbox":  roleEntry,
		"heading":  roleHeading,
	} {
		if got := atspiRole(aria); got != want {
			t.Errorf("atspiRole(%q) = %d, want %d", aria, got, want)
		}
	}
}

// An unmapped role must not become INVALID, which screen readers treat as a
// broken object rather than an unremarkable one. An empty role is a plain
// container; an unrecognised one is UNKNOWN but still a real node.
func TestUnmappedRolesStayUsable(t *testing.T) {
	if got := atspiRole(""); got != rolePanel {
		t.Errorf("empty role = %d, want panel (%d)", got, rolePanel)
	}
	if got := atspiRole("no-such-aria-role"); got == roleInvalid {
		t.Error("an unrecognised role became INVALID")
	}
}

func testTree() *a11yTree {
	return buildTree([]A11yNode{
		{ID: 1, ParentID: -1, Role: "group", X: 0, Y: 0, W: 100, H: 100},
		{ID: 2, ParentID: 1, Role: "button", X: 0, Y: 0, W: 50, H: 50},
		{ID: 3, ParentID: 1, Role: "button", X: 0, Y: 0, W: 50, H: 50}, // overlaps 2
		{ID: 4, ParentID: 2, Role: "text", X: 10, Y: 10, W: 10, H: 10},
	})
}

func TestBuildTreeOrdersChildrenByPublication(t *testing.T) {
	tr := testTree()
	kids := tr.children(1)
	if len(kids) != 2 || kids[0] != 2 || kids[1] != 3 {
		t.Errorf("children(1) = %v, want [2 3] in publication order", kids)
	}
	if len(tr.roots) != 1 || tr.roots[0] != 1 {
		t.Errorf("roots = %v, want [1]", tr.roots)
	}
}

func TestIndexInParent(t *testing.T) {
	tr := testTree()
	if got := tr.indexInParent(3); got != 1 {
		t.Errorf("indexInParent(3) = %d, want 1", got)
	}
	if got := tr.indexInParent(1); got != 0 {
		t.Errorf("indexInParent(root) = %d, want 0", got)
	}
	if got := tr.indexInParent(999); got != -1 {
		t.Errorf("indexInParent(missing) = %d, want -1", got)
	}
}

// A node whose parent was never published must stay reachable rather than
// disappearing from the tree — otherwise one bad ID hides a whole subtree.
func TestOrphanBecomesRoot(t *testing.T) {
	tr := buildTree([]A11yNode{
		{ID: 1, ParentID: -1},
		{ID: 7, ParentID: 42}, // parent 42 was never published
	})
	found := false
	for _, r := range tr.roots {
		if r == 7 {
			found = true
		}
	}
	if !found {
		t.Errorf("orphan not promoted to a root; roots = %v", tr.roots)
	}
}

// Later siblings paint over earlier ones, so a hit test must prefer the last
// match — the one the user actually sees.
func TestHitTestPrefersTopmostSibling(t *testing.T) {
	b := &atspiBridge{busName: ":test"}
	tr := testTree()
	got := b.hitTest(tr, A11yNode{}, true, 40, 40)
	if got.Path != atspiNodePrefix+"3" {
		t.Errorf("hitTest = %s, want node 3 (the later, overlapping sibling)", got.Path)
	}
}

// The deepest node containing the point wins over its ancestors.
func TestHitTestDescends(t *testing.T) {
	b := &atspiBridge{busName: ":test"}
	tr := buildTree([]A11yNode{
		{ID: 1, ParentID: -1, X: 0, Y: 0, W: 100, H: 100},
		{ID: 2, ParentID: 1, X: 0, Y: 0, W: 50, H: 50},
		{ID: 4, ParentID: 2, X: 10, Y: 10, W: 10, H: 10},
	})
	if got := b.hitTest(tr, A11yNode{}, true, 15, 15); got.Path != atspiNodePrefix+"4" {
		t.Errorf("hitTest = %s, want the deepest node 4", got.Path)
	}
}

// A point outside everything yields the null reference, which is AT-SPI's
// "nothing here" — not an error and not a stale node.
func TestHitTestMissReturnsNull(t *testing.T) {
	b := &atspiBridge{busName: ":test"}
	if got := b.hitTest(testTree(), A11yNode{}, true, 500, 500); got.Path != atspiNullPath {
		t.Errorf("hitTest off-tree = %s, want the null path", got.Path)
	}
}

func TestParsePath(t *testing.T) {
	if _, isRoot, ok := parsePath(atspiRootPath); !ok || !isRoot {
		t.Error("the application path did not parse as root")
	}
	id, isRoot, ok := parsePath(atspiNodePrefix + "12")
	if !ok || isRoot || id != 12 {
		t.Errorf("node path parsed as (%d, %v, %v), want (12, false, true)", id, isRoot, ok)
	}
	if _, _, ok := parsePath("/some/other/object"); ok {
		t.Error("an unrelated path parsed as one of ours")
	}
	if _, _, ok := parsePath(atspiNodePrefix + "notanumber"); ok {
		t.Error("a non-numeric node path parsed")
	}
}

// Only nodes that can be activated should advertise the Action interface;
// claiming one we do not answer is worse than omitting it.
func TestInterfacesFollowCapability(t *testing.T) {
	b := &atspiBridge{busName: ":test"}
	plain := b.interfacesFor(A11yNode{ID: 1, Role: "text"}, false)
	for _, i := range plain {
		if i == ifaceAction {
			t.Error("a non-tappable node advertised the Action interface")
		}
	}
	tappable := b.interfacesFor(A11yNode{ID: 2, Role: "button", Tappable: true}, false)
	found := false
	for _, i := range tappable {
		if i == ifaceAction {
			found = true
		}
	}
	if !found {
		t.Error("a tappable node did not advertise the Action interface")
	}
}
