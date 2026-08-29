package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Tree renders a hierarchy with expandable rows — a file browser, an outline,
// a settings pane.
//
// The tree owns which rows are open and nothing else. Row content is the app's
// (a Widget per node), so a tree of files and a tree of settings share the
// expansion behaviour without sharing a look. That split is why this belongs in
// widget rather than theme: what is reusable here is the folding, not the
// chevron.
type Tree struct {
	Nodes []TreeNode
	// Indent is the horizontal step per level (0 → 16).
	Indent float32
	// Expanded, when non-nil, controls which nodes are open by ID and makes the
	// tree stateless — the app owns the set and updates it in OnToggle. Leave
	// it nil to let the tree remember for itself, which is what most callers
	// want.
	Expanded map[string]bool
	// OnToggle fires when a row with children is opened or closed.
	OnToggle func(id string, expanded bool)
	// InitiallyExpanded opens these IDs on first build. Ignored when Expanded
	// is supplied, since the app is then the source of truth.
	InitiallyExpanded []string
}

// TreeNode is one row and its subtree.
type TreeNode struct {
	// ID identifies the node for expansion state, and must be unique within
	// the tree. Reusing one makes two rows fold together.
	ID string
	// Child is the row's content, excluding the disclosure affordance and
	// indentation, which the tree supplies.
	Child Widget
	// Children, when non-empty, makes the row expandable.
	Children []TreeNode
}

func (t Tree) CreateState() State { return &treeState{} }

type treeState struct {
	StateBase[Tree]
	open map[string]bool
	init bool
}

func (s *treeState) Build(ctx Ctx) Widget {
	w := s.W()
	if !s.init {
		s.init = true
		s.open = map[string]bool{}
		for _, id := range w.InitiallyExpanded {
			s.open[id] = true
		}
	}

	indent := w.Indent
	if indent <= 0 {
		indent = 16
	}
	var rows []Widget
	s.appendRows(&rows, w.Nodes, 0, indent)
	// Rows start at the leading edge. A plain Column centres its children, and
	// a centred row makes the indentation meaningless: every level would sit in
	// the middle, shifted by a step that no longer reads as depth. Indentation
	// is the only thing on screen conveying the hierarchy, so it has to be
	// measured from a fixed edge.
	col := Column(rows...)
	col.CrossAlign = layout.CrossStart
	return Semantics{Role: layout.RoleTree, Child: col}
}

// expandedSet returns whichever set is authoritative: the app's when supplied,
// otherwise the tree's own.
func (s *treeState) expandedSet() map[string]bool {
	if m := s.W().Expanded; m != nil {
		return m
	}
	return s.open
}

func (s *treeState) isOpen(id string) bool { return s.expandedSet()[id] }

// toggleDecision reports what a tap on id should do: the state it moves to, and
// whether this tree is the one that stores it.
//
// Separate from toggle because toggle must call SetState, which needs a live
// element — so the decision would otherwise only be reachable through a mounted
// widget tree, and the interesting behaviour (controlled mode not writing) would
// go untested.
func (s *treeState) toggleDecision(id string) (open, store bool) {
	return !s.isOpen(id), s.W().Expanded == nil
}

// toggle flips a row. When the app supplies Expanded it is told and left to
// update; the tree does not write into a map it does not own.
func (s *treeState) toggle(id string) {
	open, store := s.toggleDecision(id)
	if store {
		s.SetState(func() { s.open[id] = open })
	}
	if f := s.W().OnToggle; f != nil {
		f(id, open)
	}
}

// appendRows walks the tree in display order, emitting a row per visible node.
//
// Flattening rather than nesting Columns keeps the row list linear, which is
// what a virtualised list would need later and what makes the whole tree one
// Column rather than a stack of them.
func (s *treeState) appendRows(out *[]Widget, nodes []TreeNode, depth int, indent float32) {
	for _, n := range nodes {
		expandable := len(n.Children) > 0
		open := expandable && s.isOpen(n.ID)

		row := []Widget{Sized{W: indent * float32(depth)}}
		row = append(row, s.disclosureFor(n.ID, expandable, open))
		if n.Child != nil {
			row = append(row, n.Child)
		}

		// Expanded stays nil for a leaf. "Collapsed" and "has nothing to open"
		// are different announcements, and a tree that calls every leaf
		// collapsed invites the user to open rows that will never open.
		var expanded *bool
		if expandable {
			e := open
			expanded = &e
		}
		// OnActivate is what assistive technology triggers. Without it the row
		// is inert to a screen reader even though it folds on tap, because the
		// only thing carrying a handler is the chevron — a target the user
		// cannot see and, at 16pt, would struggle to hit.
		var activate func()
		if expandable {
			id := n.ID
			activate = func() { s.toggle(id) }
		}
		*out = append(*out, Semantics{
			Role:       layout.RoleTreeItem,
			Expanded:   expanded,
			OnActivate: activate,
			Child:      Row(row...),
		})

		if open {
			s.appendRows(out, n.Children, depth+1, indent)
		}
	}
}

// disclosureFor builds the affordance for one node, capturing that node's ID so
// the tap folds the row it sits beside.
//
// A leaf gets a blank of the same width rather than nothing, so its content
// lines up with its expandable siblings instead of shifting left by the width
// of a chevron — the difference between a tree and a ragged list.
func (s *treeState) disclosureFor(id string, expandable, open bool) Widget {
	const affordance = 16
	if !expandable {
		return Sized{W: affordance}
	}
	// The triangle is drawn rather than typed. The obvious spelling is a text
	// glyph — "▸" and "▾" — and it is invisible in the Go font, which has no
	// glyph for either and renders .notdef. The disclosure marker is the only
	// thing on screen saying a row can be opened, so it cannot depend on which
	// font the app happens to ship.
	//
	// Hidden from semantics as well: the row already announces its expanded
	// state, and an exposed marker is absorbed into the row's label, so a
	// screen reader would read a triangle aloud and then say nothing about
	// whether the row is open.
	return Sized{W: affordance, Child: Interactive{
		Gestures: Gestures{OnTap: func() { s.toggle(id) }},
		Child: Semantics{Hidden: true, Child: Canvas{
			W: affordance, H: affordance,
			Draw: func(c paint.Canvas, size geom.Size) {
				drawDisclosure(c, size, open)
			},
		}},
	}}
}

// drawDisclosure paints the collapsed (right-pointing) or expanded
// (down-pointing) triangle, centred in its box.
func drawDisclosure(c paint.Canvas, size geom.Size, open bool) {
	const side = 7 // the triangle's extent; the rest of the box is padding
	cx, cy := size.W/2, size.H/2
	h := float32(side) / 2

	p := paint.NewPath()
	if open {
		// Pointing down: a wide base above, apex below.
		p.MoveTo(geom.Pt{X: cx - h, Y: cy - h/2})
		p.LineTo(geom.Pt{X: cx + h, Y: cy - h/2})
		p.LineTo(geom.Pt{X: cx, Y: cy + h})
	} else {
		// Pointing right.
		p.MoveTo(geom.Pt{X: cx - h/2, Y: cy - h})
		p.LineTo(geom.Pt{X: cx + h, Y: cy})
		p.LineTo(geom.Pt{X: cx - h/2, Y: cy + h})
	}
	p.Close()
	c.FillPath(p, treeDisclosureColor)
}

// treeDisclosureColor is a mid grey that reads on light and dark surfaces
// alike. The widget layer holds no theme — a themed tree wraps this — but the
// marker still has to be visible by default.
var treeDisclosureColor = paint.Color{R: 0.45, G: 0.47, B: 0.5, A: 1}
