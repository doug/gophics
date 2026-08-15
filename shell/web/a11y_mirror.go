// This file deliberately carries no build constraint, unlike the rest of the
// package. Everything the ARIA mirror *decides* — which element to create,
// which ARIA attributes it carries, where it sits — lives here as plain data,
// so it can be tested on any platform with `go test ./shell/web/`. Only the
// application of that decision to the DOM lives in a11y_web.go behind
// js && wasm, where it can be exercised only under a JS host.
//
// The split is the point: the interesting logic is the mapping, not the
// setAttribute calls.

package web

import (
	"strconv"

	"github.com/doug/gophics/shell"
)

// mirrorElement is a described DOM element: what a11y_web.go builds.
type mirrorElement struct {
	// Tag is the element to create.
	Tag string
	// ID is the element's DOM id.
	ID string
	// Attrs are set with setAttribute, in no particular order.
	Attrs map[string]string
	// Style entries are assigned onto element.style (camelCase keys).
	Style map[string]string
	// Text, when non-empty, becomes the element's textContent.
	Text string
	// Clickable reports that the element needs an activation listener.
	Clickable bool
	// Focus reports that this element should take DOM focus.
	Focus bool
}

// describeNode maps one semantic node to the element that mirrors it. scale is
// the device pixel ratio: node bounds are physical pixels and CSS wants
// logical ones.
func describeNode(n shell.A11yNode, scale float64) mirrorElement {
	if scale <= 0 {
		scale = 1
	}
	el := mirrorElement{
		Tag:   "div",
		ID:    "gophics-a11y-" + strconv.Itoa(n.ID),
		Attrs: map[string]string{},
		Style: map[string]string{},
	}
	if n.Tappable {
		// A real button is keyboard-focusable and activatable for free, and
		// every assistive technology already knows its interaction model.
		el.Tag = "button"
		el.Clickable = true
	}

	if role := ariaRole(n.Role); role != "" {
		el.Attrs["role"] = role
	}
	if n.Label != "" {
		el.Attrs["aria-label"] = n.Label
	}
	if n.Value != "" {
		el.Attrs["aria-valuetext"] = n.Value
		// A node with a value but no name of its own reads best when the value
		// *is* the content — otherwise the AT announces an empty element.
		if !n.Tappable && n.Label == "" {
			el.Text = n.Value
		}
	}
	if n.Hint != "" {
		el.Attrs["aria-description"] = n.Hint
	}
	if n.Disabled {
		el.Attrs["aria-disabled"] = "true"
	}
	if n.Selected {
		el.Attrs["aria-selected"] = "true"
	}
	if n.Checkable {
		el.Attrs["aria-checked"] = strconv.FormatBool(n.Checked)
	}
	el.Focus = n.Focused

	// Nodes are absolutely positioned in page coordinates rather than nested
	// boxes: in gophics the semantic hierarchy and the visual geometry are
	// independent, and a child's rect is not necessarily inside its parent's.
	el.Style["position"] = "absolute"
	el.Style["left"] = px(float64(n.X) / scale)
	el.Style["top"] = px(float64(n.Y) / scale)
	el.Style["width"] = px(float64(n.W) / scale)
	el.Style["height"] = px(float64(n.H) / scale)
	el.Style["margin"] = "0"
	el.Style["padding"] = "0"
	el.Style["border"] = "0"
	el.Style["background"] = "transparent"
	el.Style["color"] = "transparent"
	el.Style["font"] = "inherit"
	el.Style["overflow"] = "hidden"
	// The mirror must never eat a real pointer event: ordinary mouse and touch
	// input belongs to the canvas and gophics's own hit testing. Assistive
	// technology activates through the accessibility API, which dispatches to
	// the element directly and is unaffected by hit testing.
	el.Style["pointerEvents"] = "none"
	return el
}

// ariaRole maps a gophics semantic role (layout.Role.String) to an ARIA role.
// The vocabularies are deliberately the same, so this is mostly a pass-through;
// it exists for the handful of names that differ and for the roles that read
// better with no role attribute at all — a bare group or text node becomes a
// landmark if you give it one, which makes a screen reader's rotor noisier
// without telling the user anything.
func ariaRole(role string) string {
	switch role {
	case "", "none", "group", "container", "text", "label":
		return ""
	case "textfield":
		return "textbox"
	case "toggle":
		return "switch"
	case "image":
		return "img"
	default:
		return role
	}
}

// parentOf returns the DOM id of the node a mirror element should be appended
// to, or "" for the mirror host itself.
func parentOf(n shell.A11yNode, known map[int]bool) string {
	if n.ParentID < 0 || !known[n.ParentID] {
		return ""
	}
	return "gophics-a11y-" + strconv.Itoa(n.ParentID)
}

func px(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) + "px" }
