//go:build windows

// Mapping gophics nodes onto UI Automation's vocabulary.
//
// The same job atspi_tree_linux.go does for AT-SPI: A11yNode carries ARIA role
// names because one vocabulary is used everywhere and mapped at the edge, and
// this is the Windows edge.
//
// UIA differs from AT-SPI in shape as well as in numbers. There is no state
// bitset — everything is a property fetched by ID, and booleans come back as
// VARIANTs. Behaviour is expressed by *patterns* rather than by an "is
// clickable" flag: a control that can be pressed supports InvokePattern, and a
// checkbox supports TogglePattern, and a screen reader decides what to say from
// which patterns are present.

package platform

// UIA control types (UIA_*ControlTypeId).
const (
	ctButton      = 50000
	ctCheckBox    = 50002
	ctComboBox    = 50003
	ctEdit        = 50004
	ctHyperlink   = 50005
	ctImage       = 50006
	ctListItem    = 50007
	ctList        = 50008
	ctMenuItem    = 50011
	ctProgressBar = 50012
	ctRadioButton = 50013
	ctScrollBar   = 50014
	ctSlider      = 50015
	ctSpinner     = 50016
	ctTab         = 50018
	ctTabItem     = 50019
	ctText        = 50020
	ctToolTip     = 50022
	ctGroup       = 50026
	ctDataGrid    = 50028
	ctWindow      = 50032
	ctPane        = 50033
	ctSeparator   = 50038
)

// UIA property IDs (UIA_*PropertyId).
const (
	propRuntimeID         = 30000
	propBoundingRectangle = 30001
	propProcessID         = 30002
	// NativeWindowHandle. Deliberately *not* implemented: answering it with
	// our own HWND makes UIA re-enter the host provider it already gave us,
	// and the tree comes back as an endless chain of identical window
	// elements. The host provider is what ties us to the HWND; this must stay
	// unanswered.
	propNativeWindowHandle    = 30020
	propControlType           = 30003
	propName                  = 30005
	propHasKeyboardFocus      = 30008
	propIsKeyboardFocusable   = 30009
	propIsEnabled             = 30010
	propAutomationID          = 30011
	propHelpText              = 30013
	propIsControlElement      = 30016
	propIsContentElement      = 30017
	propIsOffscreen           = 30022
	propToggleState           = 30086
	propIsInvokePatternAvail  = 30031
	propIsTogglePatternAvail  = 30041
	propExpandCollapseState   = 30070
	propIsExpandCollapseAvail = 30047
)

// UIA pattern IDs (UIA_*PatternId).
const (
	patternInvoke = 10000
	patternToggle = 10015
)

// ExpandCollapseState values (ExpandCollapseState enum).
const (
	expandCollapsed = 0
	expandExpanded  = 1
	expandLeafNode  = 3
)

// ToggleState values.
const (
	toggleOff = 0
	toggleOn  = 1
)

// NavigateDirection values.
const (
	navParent          = 0
	navNextSibling     = 1
	navPreviousSibling = 2
	navFirstChild      = 3
	navLastChild       = 4
)

// ariaToControlType maps the ARIA vocabulary onto UIA control types.
//
// Unmapped roles become Group rather than a "custom" type: Group is an
// ordinary container a screen reader will happily descend into, whereas a
// custom control type invites it to announce something unhelpful about an
// element it cannot classify.
var ariaToControlType = map[string]int32{
	"button":       ctButton,
	"checkbox":     ctCheckBox,
	"radio":        ctRadioButton,
	"switch":       ctCheckBox,
	"link":         ctHyperlink,
	"textbox":      ctEdit,
	"searchbox":    ctEdit,
	"slider":       ctSlider,
	"spinbutton":   ctSpinner,
	"progressbar":  ctProgressBar,
	"scrollbar":    ctScrollBar,
	"image":        ctImage,
	"img":          ctImage,
	"heading":      ctText,
	"text":         ctText,
	"label":        ctText,
	"paragraph":    ctText,
	"list":         ctList,
	"listbox":      ctList,
	"listitem":     ctListItem,
	"option":       ctListItem,
	"menuitem":     ctMenuItem,
	"combobox":     ctComboBox,
	"tab":          ctTabItem,
	"tablist":      ctTab,
	"table":        ctDataGrid,
	"grid":         ctDataGrid,
	"separator":    ctSeparator,
	"tooltip":      ctToolTip,
	"dialog":       ctWindow,
	"alertdialog":  ctWindow,
	"window":       ctWindow,
	"group":        ctGroup,
	"region":       ctGroup,
	"none":         ctPane,
	"presentation": ctPane,
}

// uiaControlType maps one node's ARIA role.
func uiaControlType(aria string) int32 {
	if ct, ok := ariaToControlType[aria]; ok {
		return ct
	}
	return ctGroup
}

// supportsInvoke reports whether a node offers InvokePattern — the pattern that
// means "this can be pressed", and what makes a screen reader offer to activate
// it.
//
// A checkbox is deliberately excluded: it toggles rather than invokes, and
// claiming both makes a reader announce two different actions for one control.
func supportsInvoke(n A11yNode) bool {
	return n.Tappable && !n.Checkable
}

// supportsToggle reports whether a node offers TogglePattern.
func supportsToggle(n A11yNode) bool { return n.Checkable }

// supportsExpandCollapse reports whether a node offers ExpandCollapsePattern —
// what UIA clients read to announce a tree item as open or closed.
func supportsExpandCollapse(n A11yNode) bool { return n.Expandable }

// expandCollapseState maps the node onto UIA's enum. LeafNode is deliberately
// not returned here: a node that is not expandable does not offer the pattern
// at all, so a client never asks. Returning LeafNode for it would advertise the
// pattern and then describe every button as an unopenable branch.
func expandCollapseState(n A11yNode) int32 {
	if n.Expanded {
		return expandExpanded
	}
	return expandCollapsed
}

// toggleState maps a node's checked flag onto UIA's tri-state.
func toggleState(n A11yNode) int32 {
	if n.Checked {
		return toggleOn
	}
	return toggleOff
}
