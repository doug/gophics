package layout

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A container is not named after what it contains. Every unlabeled node used
// to take the concatenated labels of its children as its name, so a list was
// announced with the text of all its rows before each row, and a Tree named
// itself after every visible item.
func TestContainerIsNotNamedFromItsItems(t *testing.T) {
	item := func(label string, y float32) semChild {
		return semChild{&semContainer{size: geom.Size{W: 100, H: 20}, info: SemInfo{Role: RoleListItem},
			kids: []semChild{{text(label, 100, 20), geom.Pt{}}}}, geom.Pt{Y: y}}
	}
	for _, role := range []Role{RoleList, RoleTree, RoleGroup} {
		list := &semContainer{size: geom.Size{W: 100, H: 60}, info: SemInfo{Role: role},
			kids: []semChild{item("apples", 0), item("pears", 20), item("plums", 40)}}
		nodes := CollectSemantics(list)
		if len(nodes) != 1 || nodes[0].Role != role {
			t.Fatalf("%v: nodes = %v", role, semLabels(nodes))
		}
		if nodes[0].Label != "" {
			t.Errorf("%v named from its items: %q", role, nodes[0].Label)
		}
		if got := semLabels(nodes[0].Children); len(got) != 3 || got[0] != "listitem:apples" || got[2] != "listitem:plums" {
			t.Errorf("%v children = %v, want the three items, each named from its text", role, got)
		}
	}
}

// Text inside an unlabeled container is neither its name nor absorbed: it
// stays a child, exactly as it does under a labeled one.
func TestUnlabeledContainerKeepsTextChildren(t *testing.T) {
	group := &semContainer{size: geom.Size{W: 200, H: 20}, info: SemInfo{Role: RoleGroup},
		kids: []semChild{{text("Version 1.2", 200, 20), geom.Pt{}}}}
	nodes := CollectSemantics(group)
	if len(nodes) != 1 || nodes[0].Label != "" {
		t.Fatalf("nodes = %v", semLabels(nodes))
	}
	if got := semLabels(nodes[0].Children); len(got) != 1 || got[0] != "text:Version 1.2" {
		t.Errorf("children of the unlabeled group = %v, want the text kept", got)
	}
}
