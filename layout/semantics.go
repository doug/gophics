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
	// Secure marks a text field whose content is a secret. Value then carries
	// the masked string the user sees, never the real one: a screen reader
	// must not read a password aloud, and a test harness must not be able to
	// lift it out of the semantics tree when the clipboard cannot. It is
	// carried to the accessibility bridges as A11yNode.Secure, so one that
	// can (web's mirror; a mobile host reading A11ySecure) announces "secure
	// text field" rather than the bullets.
	Secure bool
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

// Clipper is an optional Box interface for containers that paint their
// children clipped to their own bounds — a scroll viewport, above all. The
// semantics walk uses it to tell a node that has been scrolled out of view
// from one that is merely lower on the page: both are in the tree, but only
// one can be tapped, and a test that taps by label needs to know which.
// Boxes that clip only their own painting (a clipped Canvas) need not
// implement it; it is about what happens to descendants.
type Clipper interface {
	ClipsChildren() bool
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
	Secure     bool
	OnActivate func()
	Rect       geom.Rect
	// Visible is the part of Rect actually on screen: Rect narrowed to every
	// enclosing Clipper and the root. It equals Rect for a node in full view,
	// is a strip of it for a row half under the fold, and is the zero Rect
	// when Offscreen. A tap meant for the node belongs at the centre of this,
	// not of Rect — the centre of a half-hidden row is under the clip, on top
	// of whatever is drawn there.
	Visible geom.Rect
	// Offscreen is set when no part of Rect is visible: an enclosing Clipper
	// (or the root itself) clips all of it away. The node stays in the tree —
	// content further down a list exists and a test may assert so — but a tap
	// at its rect would land on whatever is drawn there instead.
	Offscreen bool
	Children  []SemNode
}

// CollectSemantics walks the laid-out tree and returns the semantics
// nodes. Boxes implementing Semantic become nodes; a node whose role names
// from content (a button, a link, a heading, a list item) and has no label
// of its own inherits the concatenated text of its subtree, while a
// container with no label stays unnamed and exposes its children.
//
// The root's own rect is the outermost clip: a page mid-slide beyond the
// window's edge is as unreachable as a row scrolled out of a viewport, and
// for the same reason.
func CollectSemantics(root Box) []SemNode {
	return collectSem(root, geom.Pt{}, geom.RectFromSize(root.Size()), false)
}

// collectSem walks b at root-space origin at. clip is the visible region in
// root space, narrowed at each Clipper on the way down; clipEmpty says it
// has closed to nothing. That is a flag rather than a property of clip
// because Intersect reports disjoint rects as the zero Rect, and the zero
// Rect is also a legitimate hairline at the origin — the two must not be
// confused once a viewport has scrolled its content wholly away.
func collectSem(b Box, at geom.Pt, clip geom.Rect, clipEmpty bool) []SemNode {
	info, isSem := SemInfo{}, false
	if s, ok := b.(Semantic); ok {
		info = s.Semantics()
		if info.Hidden {
			return nil
		}
		isSem = info.Role != RoleNone
	}

	rect := geom.Rect{Min: at, Max: at.Add(b.Size().Pt())}

	childClip, childClipEmpty := clip, clipEmpty
	if c, ok := b.(Clipper); ok && c.ClipsChildren() {
		if clipEmpty || !clip.Overlaps(rect) {
			childClip, childClipEmpty = geom.Rect{}, true
		} else {
			childClip = clip.Intersect(rect)
		}
	}
	var kids []SemNode
	if v, ok := b.(ChildVisitor); ok {
		v.VisitChildren(func(child Box, off geom.Pt) {
			kids = append(kids, collectSem(child, at.Add(off), childClip, childClipEmpty)...)
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
		Secure:     info.Secure,
		OnActivate: info.OnActivate,
		Rect:       rect,
	}
	if clipEmpty || !visibleIn(rect, clip) {
		node.Offscreen = true
	} else {
		node.Visible = clampTo(rect, clip)
	}
	// A node with no label of its own is named from its children only when
	// its role names from content. Naming every unlabeled node that way gave
	// a list the concatenated text of all its rows as its own name, read out
	// before each of them; a container stays unnamed instead.
	named := node.Label == "" && info.Role.namesFromContent()
	if named {
		node.Label = joinLabels(kids)
	}
	// Plain text children are absorbed into the node when they are its name:
	// when they supplied the label, or when the role is a control whose text
	// *is* its label (a button reading "Send" is "Send, button", not a button
	// containing a text). Structural children (buttons inside a group) always
	// stay, and so does text inside a container, labeled or not — a group
	// labeled "Settings" wrapping "Version 1.2" would otherwise make that line
	// unreadable to assistive technology: neither in the label nor in the tree.
	absorbText := named || info.Role.childrenPresentational()
	for _, k := range kids {
		if k.Role != RoleText || !absorbText {
			node.Children = append(node.Children, k)
		}
	}
	return []SemNode{node}
}

// namesFromContent reports whether a role takes its accessible name from the
// text inside it when it has none of its own — ARIA's "name from content": a
// button, a link, a heading, a tab, a checkbox, a radio, a switch, a list or
// tree item, and plain text. Containers (group, list, tree) do not: a list
// named after every row in it would be announced with all of them before each
// one. Nor do the value-bearing controls (text field, slider, progress bar)
// and images, whose text is a value or a caption, not a name.
func (r Role) namesFromContent() bool {
	switch r {
	case RoleText, RoleButton, RoleLink, RoleHeading, RoleCheckbox, RoleRadio,
		RoleSwitch, RoleListItem, RoleTab, RoleTreeItem:
		return true
	}
	return false
}

// childrenPresentational reports whether a role's descendants are part of its
// own presentation rather than content of their own — ARIA's
// childrenPresentational: a button, a checkbox, a slider, a tab. Assistive
// technology names such a control from its text and does not descend into
// it. Containers (groups, lists, links, headings) expose what is inside them.
func (r Role) childrenPresentational() bool {
	switch r {
	case RoleButton, RoleTextField, RoleImage, RoleCheckbox, RoleRadio,
		RoleSwitch, RoleSlider, RoleProgress, RoleTab:
		return true
	}
	return false
}

// visibleIn reports whether any part of r lies inside clip. A zero-extent
// axis counts as inside when its coordinate does — a hairline separator on
// the fold is still on screen — while a rect with extent that merely touches
// the clip's edge is not: nothing of it would paint.
func visibleIn(r, clip geom.Rect) bool {
	return spanVisible(r.Min.X, r.Max.X, clip.Min.X, clip.Max.X) &&
		spanVisible(r.Min.Y, r.Max.Y, clip.Min.Y, clip.Max.Y)
}

func spanVisible(lo, hi, clo, chi float32) bool {
	if hi <= lo {
		return lo >= clo && lo <= chi
	}
	return lo < chi && hi > clo
}

// clampTo is r cut down to clip, for an r known to be visible in it. Unlike
// Intersect it keeps a zero-extent result — the hairline visibleIn admits —
// at its coordinate instead of collapsing it to the zero Rect.
func clampTo(r, clip geom.Rect) geom.Rect {
	r.Min.X = max(r.Min.X, clip.Min.X)
	r.Min.Y = max(r.Min.Y, clip.Min.Y)
	r.Max.X = min(r.Max.X, clip.Max.X)
	r.Max.Y = min(r.Max.Y, clip.Max.Y)
	return r
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
