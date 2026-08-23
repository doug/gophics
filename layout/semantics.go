package layout

import (
	"strings"

	"github.com/doug/gophics/geom"
)

// This file is the accessibility foundation: a semantics
// tree collected from the laid-out box tree, testable headless. Platform
// bridges (AccessKit-style) consume these nodes later; correctness is
// assertable long before OS integration exists.

// Role classifies a semantic node for assistive technology.
type Role uint8

// The role names are the ARIA vocabulary, because every platform's
// accessibility API can be reached from it and using one spelling everywhere
// keeps the bridges free of per-platform translation tables.
//
// New roles are appended, never inserted: the numeric values are not part of
// any serialized format, but keeping them stable makes a mismatched build
// obvious rather than silently mislabeling controls.
const (
	RoleNone Role = iota
	RoleText
	RoleButton
	RoleTextField
	RoleGroup
	RoleImage
	RoleLink
	RoleHeading
	RoleCheckbox
	RoleRadio
	RoleSwitch
	RoleSlider
	RoleProgress
	RoleList
	RoleListItem
	RoleTab
	RoleTree
	RoleTreeItem
)

func (r Role) String() string {
	switch r {
	case RoleText:
		return "text"
	case RoleButton:
		return "button"
	case RoleTextField:
		return "textfield"
	case RoleGroup:
		return "group"
	case RoleImage:
		return "img"
	case RoleLink:
		return "link"
	case RoleHeading:
		return "heading"
	case RoleCheckbox:
		return "checkbox"
	case RoleRadio:
		return "radio"
	case RoleSwitch:
		return "switch"
	case RoleSlider:
		return "slider"
	case RoleProgress:
		return "progressbar"
	case RoleList:
		return "list"
	case RoleListItem:
		return "listitem"
	case RoleTab:
		return "tab"
	case RoleTree:
		return "tree"
	case RoleTreeItem:
		return "treeitem"
	}
	return "none"
}

// SemInfo is the semantic description a box contributes.
type SemInfo struct {
	Role    Role
	Label   string
	Value   string
	Focused bool
	// Hidden prunes this box and its subtree from the semantics tree
	// (decorative content).
	Hidden bool
	// OnActivate, if set, is invoked when assistive technology activates
	// the node (VoiceOver double-tap, TalkBack activate). Usually the same
	// action as OnTap.
	OnActivate func()
	// Checked is set for toggleable nodes (checkbox, switch); nil = not
	// checkable. Disabled and Selected mirror the ARIA states.
	Checked  *bool
	Disabled bool
	Selected bool
	// Expanded is set for nodes that open and close (tree items, disclosure
	// rows); nil = not expandable. Same shape as Checked, and for the same
	// reason: "collapsed" and "cannot expand" are different things to announce.
	Expanded *bool
	// Hint describes the result of activating the node ("opens the thread").
	Hint string
}

// Semantic is implemented by boxes that contribute semantics.
type Semantic interface {
	Semantics() SemInfo
}

// ChildVisitor exposes a box's children and their offsets for tree walks
// (semantics, inspectors). Leaf boxes need not implement it.
type ChildVisitor interface {
	VisitChildren(visit func(child Box, offset geom.Pt))
}

// SemNode is one node of the collected semantics tree, with its rect in
// root coordinates.
type SemNode struct {
	Role       Role
	Label      string
	Value      string
	Focused    bool
	Disabled   bool
	Selected   bool
	Checked    *bool
	Expanded   *bool
	Hint       string
	OnActivate func()
	Rect       geom.Rect
	Children   []SemNode
}

// CollectSemantics walks the laid-out tree and returns the semantics
// nodes. Boxes implementing Semantic become nodes; an interactive node
// with no label of its own inherits the concatenated text of its subtree.
func CollectSemantics(root Box) []SemNode {
	return collectSem(root, geom.Pt{})
}

func collectSem(b Box, at geom.Pt) []SemNode {
	info, isSem := SemInfo{}, false
	if s, ok := b.(Semantic); ok {
		info = s.Semantics()
		if info.Hidden {
			return nil
		}
		isSem = info.Role != RoleNone
	}

	var kids []SemNode
	if v, ok := b.(ChildVisitor); ok {
		v.VisitChildren(func(child Box, off geom.Pt) {
			kids = append(kids, collectSem(child, at.Add(off))...)
		})
	}

	if !isSem {
		return kids
	}
	node := SemNode{
		Role:       info.Role,
		Label:      info.Label,
		Value:      info.Value,
		Focused:    info.Focused,
		Disabled:   info.Disabled,
		Selected:   info.Selected,
		Checked:    info.Checked,
		Expanded:   info.Expanded,
		Hint:       info.Hint,
		OnActivate: info.OnActivate,
		Rect:       geom.Rect{Min: at, Max: at.Add(b.Size().Pt())},
	}
	if node.Label == "" {
		node.Label = joinLabels(kids)
	}
	// Plain text children are absorbed into the labeled node; structural
	// children (buttons inside a group, etc.) stay.
	for _, k := range kids {
		if k.Role != RoleText {
			node.Children = append(node.Children, k)
		}
	}
	return []SemNode{node}
}

func joinLabels(nodes []SemNode) string {
	var parts []string
	for _, n := range nodes {
		if n.Label != "" {
			parts = append(parts, n.Label)
		}
	}
	return strings.Join(parts, " ")
}

// FlattenSemantics returns the tree as a depth-first list (for assertions
// and simple consumers).
func FlattenSemantics(nodes []SemNode) []SemNode {
	var out []SemNode
	for _, n := range nodes {
		kids := n.Children
		n.Children = nil
		out = append(out, n)
		out = append(out, FlattenSemantics(kids)...)
	}
	return out
}

// VisitChildren implementations for the built-in containers.

func (b *Padded) VisitChildren(visit func(Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, b.childOffset())
	}
}

func (b *Aligned) VisitChildren(visit func(Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, b.offset)
	}
}

func (b *Sized) VisitChildren(visit func(Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (b *Decorated) VisitChildren(visit func(Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (f *Flex) VisitChildren(visit func(Box, geom.Pt)) {
	for i, c := range f.Children {
		if i >= len(f.offsets) {
			break
		}
		visit(c.Box, f.offsets[i])
	}
}

func (v *Viewport) VisitChildren(visit func(Box, geom.Pt)) {
	if v.Child != nil {
		visit(v.Child, v.scrollPt())
	}
}

// Semantics implements Semantic for text boxes.
func (b *TextBox) Semantics() SemInfo {
	return SemInfo{Role: RoleText, Label: b.Text}
}
