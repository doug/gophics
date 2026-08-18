package platform

import "sort"

// a11yTree indexes a published node list for the lookups every accessibility
// bridge makes: by ID, and parent → ordered children.
//
// Shared rather than per-platform. AT-SPI and UI Automation disagree about
// almost everything — numeric roles versus properties, state bitsets versus
// patterns — but they ask the same questions of the tree itself, and the
// answers are the same shape. Keeping one copy also means the ordering rules
// below are decided once instead of drifting apart.
type a11yTree struct {
	nodes []A11yNode
	byID  map[int]A11yNode
	kids  map[int][]int
	roots []int // nodes whose parent is -1; normally exactly one
}

// buildTree indexes a flat node list.
//
// Order within a parent follows the order the nodes were published, which is
// creation order — the order the app draws them, and so the order they should
// be read in.
func buildTree(nodes []A11yNode) *a11yTree {
	t := &a11yTree{
		nodes: nodes,
		byID:  make(map[int]A11yNode, len(nodes)),
		kids:  make(map[int][]int),
	}
	for _, n := range nodes {
		t.byID[n.ID] = n
	}
	for _, n := range nodes {
		if n.ParentID == -1 {
			t.roots = append(t.roots, n.ID)
			continue
		}
		// A node whose parent was not published would otherwise vanish; treat
		// it as a root so it stays reachable. One bad ID should not hide a
		// whole subtree from a screen reader.
		if _, ok := t.byID[n.ParentID]; !ok {
			t.roots = append(t.roots, n.ID)
			continue
		}
		t.kids[n.ParentID] = append(t.kids[n.ParentID], n.ID)
	}
	sort.Ints(t.roots)
	return t
}

// children returns the ordered child IDs of a node.
func (t *a11yTree) children(id int) []int { return t.kids[id] }

// siblings returns the ordered sibling list containing id, and id's position
// within it, or -1 if it is not there.
func (t *a11yTree) siblings(id int) ([]int, int) {
	n, ok := t.byID[id]
	if !ok {
		return nil, -1
	}
	list := t.roots
	if n.ParentID != -1 {
		if _, ok := t.byID[n.ParentID]; ok {
			list = t.kids[n.ParentID]
		}
	}
	for i, s := range list {
		if s == id {
			return list, i
		}
	}
	return list, -1
}

// indexInParent is what an assistive technology uses to walk back up and to
// order siblings.
func (t *a11yTree) indexInParent(id int) int32 {
	n, ok := t.byID[id]
	if !ok {
		return -1
	}
	if n.ParentID == -1 {
		return 0 // the sole child of the application/window object
	}
	if _, i := t.siblings(id); i >= 0 {
		return int32(i)
	}
	return -1
}
