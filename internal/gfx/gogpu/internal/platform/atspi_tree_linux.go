//go:build linux

// The accessible tree: gophics nodes seen as AT-SPI objects.
//
// A11yNode carries ARIA role names, because one vocabulary is used across every
// platform and mapped at the edge (see a11y.go). This is that edge for Linux:
// ARIA names become AT-SPI's numeric role enum, and the assorted booleans
// become its state bitset.
//
// The numbers are not guesses. They were read out of pyatspi on a machine with
// at-spi2 installed, which is the same source the screen reader compiles
// against — several are unobvious (CHECKABLE is 41, not adjacent to CHECKED at
// 4), and a wrong value does not fail loudly. It makes a button announce as
// something else.

package platform

import "sort"

// AT-SPI object paths. The application object lives at .../accessible/root and
// every node hangs beneath it by ID.
const (
	atspiRootPath   = "/org/a11y/atspi/accessible/root"
	atspiNodePrefix = "/org/a11y/atspi/accessible/"
	atspiNullPath   = "/org/a11y/atspi/null"
)

// AT-SPI roles (AtspiRole), read from pyatspi.
const (
	roleInvalid      uint32 = 0
	roleCheckBox     uint32 = 7
	roleComboBox     uint32 = 11
	roleDialog       uint32 = 16
	roleFiller       uint32 = 20
	roleFrame        uint32 = 23
	roleImage        uint32 = 27
	roleLabel        uint32 = 29
	roleList         uint32 = 31
	roleListItem     uint32 = 32
	roleMenuItem     uint32 = 35
	rolePageTab      uint32 = 37
	rolePageTabList  uint32 = 38
	rolePanel        uint32 = 39
	roleProgressBar  uint32 = 42
	rolePushButton   uint32 = 43
	roleRadioButton  uint32 = 44
	roleScrollBar    uint32 = 48
	roleSeparator    uint32 = 50
	roleSlider       uint32 = 51
	roleSpinButton   uint32 = 52
	roleTable        uint32 = 55
	roleText         uint32 = 61
	roleToggleButton uint32 = 62
	roleToolTip      uint32 = 64
	roleUnknown      uint32 = 67
	roleWindow       uint32 = 69
	roleApplication  uint32 = 75
	roleEntry        uint32 = 79
	roleHeading      uint32 = 83
	roleLink         uint32 = 88
)

// AT-SPI states (AtspiStateType), read from pyatspi.
const (
	stateActive     = 1
	stateChecked    = 4
	stateEnabled    = 8
	stateFocusable  = 11
	stateFocused    = 12
	stateSelectable = 22
	stateSelected   = 23
	stateSensitive  = 24
	stateShowing    = 25
	stateVisible    = 30
	stateCheckable  = 41
)

// ariaToRole maps the ARIA vocabulary A11yNode uses onto AT-SPI's enum.
// Unlisted roles fall through to a generic one rather than to INVALID, which
// screen readers treat as a broken object rather than an unremarkable one.
var ariaToRole = map[string]uint32{
	"button":       rolePushButton,
	"checkbox":     roleCheckBox,
	"radio":        roleRadioButton,
	"switch":       roleToggleButton,
	"link":         roleLink,
	"textbox":      roleEntry,
	"searchbox":    roleEntry,
	"slider":       roleSlider,
	"spinbutton":   roleSpinButton,
	"progressbar":  roleProgressBar,
	"scrollbar":    roleScrollBar,
	"image":        roleImage,
	"img":          roleImage,
	"heading":      roleHeading,
	"list":         roleList,
	"listitem":     roleListItem,
	"listbox":      roleList,
	"option":       roleListItem,
	"menuitem":     roleMenuItem,
	"combobox":     roleComboBox,
	"tab":          rolePageTab,
	"tablist":      rolePageTabList,
	"table":        roleTable,
	"grid":         roleTable,
	"separator":    roleSeparator,
	"tooltip":      roleToolTip,
	"dialog":       roleDialog,
	"alertdialog":  roleDialog,
	"window":       roleWindow,
	"text":         roleLabel,
	"label":        roleLabel,
	"paragraph":    roleLabel,
	"group":        rolePanel,
	"region":       rolePanel,
	"none":         roleFiller,
	"presentation": roleFiller,
	"application":  roleApplication,
}

// roleNames is the human-readable name for each role we emit, as AT-SPI spells
// it (lowercase, space-separated). GetRoleName returns this.
var roleNames = map[uint32]string{
	roleInvalid: "invalid", roleCheckBox: "check box", roleComboBox: "combo box",
	roleDialog: "dialog", roleFiller: "filler", roleFrame: "frame",
	roleImage: "image", roleLabel: "label", roleList: "list",
	roleListItem: "list item", roleMenuItem: "menu item", rolePageTab: "page tab",
	rolePageTabList: "page tab list", rolePanel: "panel",
	roleProgressBar: "progress bar", rolePushButton: "push button",
	roleRadioButton: "radio button", roleScrollBar: "scroll bar",
	roleSeparator: "separator", roleSlider: "slider",
	roleSpinButton: "spin button", roleTable: "table", roleText: "text",
	roleToggleButton: "toggle button", roleToolTip: "tool tip",
	roleUnknown: "unknown", roleWindow: "window", roleApplication: "application",
	roleEntry: "entry", roleHeading: "heading", roleLink: "link",
}

// atspiRole maps one node's ARIA role name.
func atspiRole(aria string) uint32 {
	if r, ok := ariaToRole[aria]; ok {
		return r
	}
	// An unknown role is still a real object with a label and bounds, so
	// "panel" (a plain container) reads better than "unknown".
	if aria == "" {
		return rolePanel
	}
	return roleUnknown
}

func atspiRoleName(r uint32) string {
	if n, ok := roleNames[r]; ok {
		return n
	}
	return "unknown"
}

// atspiStates builds the two-word state bitset for a node.
//
// SHOWING and VISIBLE are what make a node appear at all — a screen reader
// filters out anything lacking them — so they are set for every published
// node. Disabled clears SENSITIVE and ENABLED rather than adding a state,
// which is how AT-SPI expresses it.
func atspiStates(n A11yNode) []uint32 {
	var bits [2]uint32
	set := func(b uint) { bits[b/32] |= 1 << (b % 32) }

	set(stateVisible)
	set(stateShowing)
	if !n.Disabled {
		set(stateEnabled)
		set(stateSensitive)
	}
	if n.Tappable {
		set(stateFocusable)
	}
	if n.Focused {
		set(stateFocused)
	}
	if n.Checkable {
		set(stateCheckable)
	}
	if n.Checked {
		set(stateChecked)
	}
	if n.Selected {
		set(stateSelectable)
		set(stateSelected)
	}
	return bits[:]
}

// a11yTree is a snapshot of the published nodes, indexed for the lookups
// AT-SPI makes: by ID, and parent → ordered children.
type a11yTree struct {
	nodes []A11yNode
	byID  map[int]A11yNode
	kids  map[int][]int
	roots []int // nodes whose parent is -1; normally exactly one
}

// buildTree indexes a flat node list. Order within a parent follows the order
// the nodes were published, which is creation order — the order the app draws
// them, and so the order they should be read in.
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
		// it as a root so it stays reachable.
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

// indexInParent is what AT-SPI uses to walk back up and to order siblings.
func (t *a11yTree) indexInParent(id int) int32 {
	n, ok := t.byID[id]
	if !ok {
		return -1
	}
	if n.ParentID == -1 {
		return 0 // the sole child of the application object
	}
	for i, sib := range t.kids[n.ParentID] {
		if sib == id {
			return int32(i)
		}
	}
	return -1
}
