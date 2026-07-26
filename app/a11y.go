package app

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
)

// A11yNode is one accessibility node in a flat, ID-addressed tree — the
// shape platform screen-reader bridges (Android AccessibilityNodeProvider,
// iOS UIAccessibilityElement, AccessKit) consume. Rects are in physical
// pixels. Built from the semantics tree (Core.Semantics).
type A11yNode struct {
	ID       int
	ParentID int // -1 for the root
	Role     string
	Label    string
	Value    string
	Hint     string
	// X, Y, W, H are physical-pixel bounds.
	X, Y, W, H int
	Tappable   bool
	Focused    bool
	Disabled   bool
	Selected   bool
	Checkable  bool
	Checked    bool
	Children   []int
}

// a11yTree is the cached flattening plus the activation callbacks by ID.
type a11yTree struct {
	nodes   []A11yNode
	byID    map[int]int // node ID → index
	actions map[int]func()
}

// A11yTree flattens the current semantics tree into ID-addressed nodes at
// the given device scale (physical px). Call after a frame. The activation
// callbacks are retained so A11yActivate can invoke them.
func (c *Core) A11yTree(scale float32) []A11yNode {
	t := &a11yTree{byID: map[int]int{}, actions: map[int]func(){}}
	sem := c.Semantics()
	next := 0
	var walk func(n layout.SemNode, parent int) int
	walk = func(n layout.SemNode, parent int) int {
		id := next
		next++
		node := A11yNode{
			ID: id, ParentID: parent,
			Role: n.Role.String(), Label: n.Label, Value: n.Value, Hint: n.Hint,
			X: int(n.Rect.Min.X * scale), Y: int(n.Rect.Min.Y * scale),
			W: int(n.Rect.Dx() * scale), H: int(n.Rect.Dy() * scale),
			Tappable: n.OnActivate != nil,
			Focused:  n.Focused, Disabled: n.Disabled, Selected: n.Selected,
			Checkable: n.Checked != nil, Checked: n.Checked != nil && *n.Checked,
		}
		idx := len(t.nodes)
		t.nodes = append(t.nodes, node)
		t.byID[id] = idx
		if n.OnActivate != nil {
			t.actions[id] = n.OnActivate
		}
		for _, ch := range n.Children {
			cid := walk(ch, id)
			t.nodes[idx].Children = append(t.nodes[idx].Children, cid)
		}
		return id
	}
	// A synthetic root groups the top-level semantic nodes so the host has
	// a single container to attach to.
	rootIdx := len(t.nodes)
	t.nodes = append(t.nodes, A11yNode{ID: next, ParentID: -1, Role: "group",
		W: int(c.size.W * scale), H: int(c.size.H * scale)})
	rootID := next
	t.byID[rootID] = rootIdx
	next++
	for _, n := range sem {
		cid := walk(n, rootID)
		t.nodes[rootIdx].Children = append(t.nodes[rootIdx].Children, cid)
	}
	c.a11y = t
	return t.nodes
}

// A11yActivate invokes the activation action of the node (screen-reader
// activate). Safe to call with an unknown or non-actionable ID.
func (c *Core) A11yActivate(id int) {
	if c.a11y == nil {
		return
	}
	if fn := c.a11y.actions[id]; fn != nil {
		fn()
	}
}

// A11yHitTest returns the ID of the deepest tappable/labeled node at the
// physical-pixel point, or -1 — for explore-by-touch.
func (c *Core) A11yHitTest(xPx, yPx int, scale float32) int {
	if c.a11y == nil {
		return -1
	}
	p := geom.Pt{X: float32(xPx), Y: float32(yPx)}
	best := -1
	bestArea := float32(1e18)
	for _, n := range c.a11y.nodes {
		r := geom.RectXYWH(float32(n.X), float32(n.Y), float32(n.W), float32(n.H))
		if r.Contains(p) && (n.Label != "" || n.Tappable) {
			if a := r.Dx() * r.Dy(); a < bestArea {
				best, bestArea = n.ID, a
			}
		}
	}
	return best
}
