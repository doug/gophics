package app

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
)

// A11yNode is one accessibility node in a flat, ID-addressed tree — the
// shape platform screen-reader bridges (Android AccessibilityNodeProvider,
// iOS UIAccessibilityElement, AccessKit) consume. Rects are in physical
// pixels. Built from the semantics tree (core.Semantics).
//
// It is an alias for shell.A11yNode so the tree the app flattens is the same
// value a platform bridge publishes, with no copy or conversion between the
// layers.
type A11yNode = shell.A11yNode

// a11yTree is the cached flattening plus the activation callbacks by ID.
type a11yTree struct {
	nodes   []A11yNode
	byID    map[int]int // node ID → index
	actions map[int]func()
}

// A11yTree flattens the current semantics tree into ID-addressed nodes at
// the given device scale (physical px). Call after a frame. The activation
// callbacks are retained so A11yActivate can invoke them.
func (c *core) A11yTree(scale float32) []A11yNode {
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
			Expandable: n.Expanded != nil, Expanded: n.Expanded != nil && *n.Expanded,
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

// publishA11y hands the current tree to the platform accessibility bridge, if
// the window has one. A screen reader is a second renderer with its own frame
// budget: rebuilding its tree on every frame would churn the platform's node
// cache (and, on web, the DOM) for animations that change no semantics at all,
// so the flattened tree is diffed and only a real change is published.
func (h *shellHandler) publishA11y() {
	a := h.core.Owner.Accessibility
	if a == nil {
		return
	}
	nodes := h.core.A11yTree(h.core.lastScale)
	if a11yTreeEqual(h.lastA11y, nodes) {
		return
	}
	// A11yTree hands back its own slice each call, so retaining it is safe.
	h.lastA11y = nodes
	a.SetTree(nodes, h.core.A11yActivate)
}

// a11yTreeEqual compares two flattened trees field by field. A11yNode holds
// only comparable fields plus the Children ID slice, so this is a cheap
// structural equality — no hashing, no allocation.
func a11yTreeEqual(a, b []A11yNode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.ID != y.ID || x.ParentID != y.ParentID || x.Role != y.Role ||
			x.Label != y.Label || x.Value != y.Value || x.Hint != y.Hint ||
			x.X != y.X || x.Y != y.Y || x.W != y.W || x.H != y.H ||
			x.Tappable != y.Tappable || x.Focused != y.Focused ||
			x.Disabled != y.Disabled || x.Selected != y.Selected ||
			x.Checkable != y.Checkable || x.Checked != y.Checked {
			return false
		}
		if len(x.Children) != len(y.Children) {
			return false
		}
		for j := range x.Children {
			if x.Children[j] != y.Children[j] {
				return false
			}
		}
	}
	return true
}

// A11yActivate invokes the activation action of the node (screen-reader
// activate). Safe to call with an unknown or non-actionable ID.
func (c *core) A11yActivate(id int) {
	if c.a11y == nil {
		return
	}
	if fn := c.a11y.actions[id]; fn != nil {
		fn()
	}
}

// A11yHitTest returns the ID of the deepest tappable/labeled node at the
// physical-pixel point, or -1 — for explore-by-touch.
func (c *core) A11yHitTest(xPx, yPx int, scale float32) int {
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

// The app's handler is the A11yProvider every native bridge codes against.
// Asserting it here means a signature drift breaks this build rather than a
// platform bridge that only compiles on one GOOS.
var _ shell.A11yProvider = (*shellHandler)(nil)
