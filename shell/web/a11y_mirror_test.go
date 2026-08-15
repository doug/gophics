// No build constraint, matching a11y_mirror.go: these run on every platform,
// so the ARIA mapping is covered by an ordinary `go test ./...` rather than
// only under a browser.

package web

import (
	"testing"

	"github.com/doug/gophics/shell"
)

func TestAriaRole(t *testing.T) {
	cases := map[string]string{
		// Roles ARIA has no useful equivalent for produce no role attribute:
		// making every group a landmark clutters a screen reader's rotor.
		"":          "",
		"none":      "",
		"group":     "",
		"text":      "",
		"label":     "",
		"container": "",
		// The names that differ between the two vocabularies.
		"textfield": "textbox",
		"toggle":    "switch",
		"image":     "img",
		// Everything already spelled the ARIA way passes through untouched.
		"button":      "button",
		"checkbox":    "checkbox",
		"radio":       "radio",
		"switch":      "switch",
		"slider":      "slider",
		"progressbar": "progressbar",
		"heading":     "heading",
		"link":        "link",
		"list":        "list",
		"listitem":    "listitem",
		"tab":         "tab",
		"img":         "img",
	}
	for in, want := range cases {
		if got := ariaRole(in); got != want {
			t.Errorf("ariaRole(%q) = %q, want %q", in, got, want)
		}
	}
}

// A tappable node has to become something an AT can operate, not a div.
func TestDescribeTappableIsAButton(t *testing.T) {
	d := describeNode(shell.A11yNode{
		ID: 3, Role: "button", Label: "Send", Tappable: true,
		X: 20, Y: 40, W: 100, H: 44,
	}, 2)

	if d.Tag != "button" {
		t.Errorf("tag = %q, want button", d.Tag)
	}
	if !d.Clickable {
		t.Error("tappable node got no activation listener")
	}
	if d.ID != "gophics-a11y-3" {
		t.Errorf("id = %q", d.ID)
	}
	if d.Attrs["aria-label"] != "Send" {
		t.Errorf("aria-label = %q, want Send", d.Attrs["aria-label"])
	}
	// Physical pixels in, CSS pixels out.
	if d.Style["left"] != "10.00px" || d.Style["top"] != "20.00px" {
		t.Errorf("position = (%s,%s), want (10.00px,20.00px) at dpr 2",
			d.Style["left"], d.Style["top"])
	}
	if d.Style["width"] != "50.00px" || d.Style["height"] != "22.00px" {
		t.Errorf("size = (%s,%s), want (50.00px,22.00px) at dpr 2",
			d.Style["width"], d.Style["height"])
	}
	// The mirror must stay transparent to real pointer input.
	if d.Style["pointerEvents"] != "none" {
		t.Errorf("pointerEvents = %q, want none", d.Style["pointerEvents"])
	}
}

// State has to reach the AT, not just the role: an unchecked checkbox that
// announces no state is indistinguishable from a checked one.
func TestDescribePublishesState(t *testing.T) {
	on := describeNode(shell.A11yNode{
		Role: "checkbox", Label: "Remember me", Tappable: true,
		Checkable: true, Checked: true,
	}, 1)
	if on.Attrs["aria-checked"] != "true" {
		t.Errorf("aria-checked = %q, want true", on.Attrs["aria-checked"])
	}

	off := describeNode(shell.A11yNode{
		Role: "checkbox", Checkable: true, Checked: false,
	}, 1)
	if off.Attrs["aria-checked"] != "false" {
		t.Errorf("aria-checked = %q, want false", off.Attrs["aria-checked"])
	}

	// A node that is not checkable must not claim a checked state at all.
	plain := describeNode(shell.A11yNode{Role: "button", Tappable: true}, 1)
	if _, ok := plain.Attrs["aria-checked"]; ok {
		t.Error("non-checkable node published aria-checked")
	}

	dis := describeNode(shell.A11yNode{Role: "button", Disabled: true, Selected: true}, 1)
	if dis.Attrs["aria-disabled"] != "true" || dis.Attrs["aria-selected"] != "true" {
		t.Errorf("state attrs = %v", dis.Attrs)
	}
}

func TestDescribeValueAndHint(t *testing.T) {
	sl := describeNode(shell.A11yNode{Role: "slider", Label: "Volume", Value: "25%"}, 1)
	if sl.Attrs["aria-valuetext"] != "25%" {
		t.Errorf("aria-valuetext = %q", sl.Attrs["aria-valuetext"])
	}
	// It has a label, so the value must not also become the text content or
	// the AT reads the number twice.
	if sl.Text != "" {
		t.Errorf("labeled node also got textContent %q", sl.Text)
	}

	// An unlabeled, non-interactive value node reads best as content.
	txt := describeNode(shell.A11yNode{Role: "text", Value: "Inbox"}, 1)
	if txt.Text != "Inbox" {
		t.Errorf("textContent = %q, want Inbox", txt.Text)
	}

	h := describeNode(shell.A11yNode{Role: "button", Hint: "opens the thread", Tappable: true}, 1)
	if h.Attrs["aria-description"] != "opens the thread" {
		t.Errorf("aria-description = %q", h.Attrs["aria-description"])
	}
}

// A zero or missing device pixel ratio must not collapse the whole mirror to
// a point.
func TestDescribeToleratesBadScale(t *testing.T) {
	d := describeNode(shell.A11yNode{W: 100, H: 50}, 0)
	if d.Style["width"] != "100.00px" {
		t.Errorf("width at scale 0 = %q, want 100.00px", d.Style["width"])
	}
}

func TestParentOf(t *testing.T) {
	known := map[int]bool{0: true, 1: true}
	if got := parentOf(shell.A11yNode{ID: 1, ParentID: 0}, known); got != "gophics-a11y-0" {
		t.Errorf("parentOf child = %q", got)
	}
	// The root attaches to the mirror host.
	if got := parentOf(shell.A11yNode{ID: 0, ParentID: -1}, known); got != "" {
		t.Errorf("parentOf root = %q, want the host", got)
	}
	// A node whose parent is not in this tree must not be dropped on the
	// floor — it falls back to the host rather than vanishing from the AT.
	if got := parentOf(shell.A11yNode{ID: 5, ParentID: 99}, known); got != "" {
		t.Errorf("parentOf orphan = %q, want the host", got)
	}
}

// The focused node is the one the AT should land on after a republish.
func TestDescribeFocusFollowsGophics(t *testing.T) {
	if !describeNode(shell.A11yNode{Role: "textfield", Focused: true}, 1).Focus {
		t.Error("focused node not marked for DOM focus")
	}
	if describeNode(shell.A11yNode{Role: "textfield"}, 1).Focus {
		t.Error("unfocused node marked for DOM focus")
	}
}
