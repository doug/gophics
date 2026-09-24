package widget_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// A password field paints bullets whether or not it wraps.
//
// The single-line painter went through shown(), which masks; the multiline
// painter and the layout measurement read the editor's text directly, so a
// Multiline Obscure field drew its secret in the clear.
func TestObscureFieldPaintsBulletsInBothLayouts(t *testing.T) {
	for _, multi := range []bool{false, true} {
		h := headless(t, widget.Sized{W: 300, Child: widget.TextField{
			Value: "secretword", Obscure: true, Multiline: multi,
		}}, 320, 240)
		h.Render()
		scene := fmt.Sprintf("%+v", h.Scene())
		if strings.Contains(scene, "secretword") {
			t.Errorf("Multiline=%v: an Obscure field painted its plaintext", multi)
		}
		if !strings.Contains(scene, "••••") {
			t.Errorf("Multiline=%v: an Obscure field painted no bullets", multi)
		}
	}
}

// Disabling a field that holds focus releases the focus.
//
// A Disabled field swaps in empty Gestures, and the focus bookkeeping in
// Interactive returned early for anything not focusable — so KeyboardTarget
// kept pointing at the now-empty handlers: no OnFocus(false), the caret kept
// blinking, the semantics said Focused and Disabled at once, the soft
// keyboard was never hidden, and nothing else could autofocus.
func TestDisablingAFocusedFieldReleasesFocus(t *testing.T) {
	disabled := false
	var focus []bool
	root, m := newMutable(func(widget.Ctx) widget.Widget {
		return widget.TextField{Value: "x", Autofocus: true, Disabled: disabled,
			OnFocus: func(v bool) { focus = append(focus, v) }}
	})
	h := headless(t, root, 320, 240)
	h.Render()
	if len(focus) != 1 || !focus[0] {
		t.Fatalf("autofocus did not take: focus log %v", focus)
	}

	disabled = true
	m.Rebuild()
	h.Render()
	if len(focus) != 2 || focus[1] {
		t.Errorf("focus log %v; want OnFocus(false) when the field became Disabled", focus)
	}
	if h.Owner().KeyboardTarget != nil {
		t.Error("KeyboardTarget still points at the disabled field")
	}
	if n := findRole(h.Semantics(), layout.RoleTextField); n != nil && n.Focused {
		t.Error("semantics report a Disabled field as Focused")
	}
}

// A ReadOnly field's menu offers what it can do — Copy and Select All — and
// neither Cut nor Paste.
//
// The keyboard shortcuts checked editable; the menu did not, so Cut from a
// long-press menu edited a field the app had marked read-only, and Paste read
// the clipboard (an iOS "pasted from" notice) for a paste that could not land.
func TestReadOnlyFieldMenuOffersNoCutOrPaste(t *testing.T) {
	var changes []string
	h := headless(t, widget.OverlayHost{Child: widget.Sized{W: 300, Child: widget.TextField{
		Value: "hello world", ReadOnly: true,
		OnChange: func(s string) { changes = append(changes, s) },
	}}}, 320, 240)
	h.Clipboard().S = "pasteable"
	h.Render()
	at := geom.Pt{X: 20, Y: 8}
	h.TouchPress(at)
	h.Step(0.7) // past the long-press threshold
	h.TouchRelease(at)
	h.Render()

	if !hasLabel(h, "Copy") {
		t.Fatalf("no edit menu after a long press; labels: %v", labels(h))
	}
	for _, forbidden := range []string{"Cut", "Paste"} {
		if hasLabel(h, forbidden) {
			t.Errorf("a ReadOnly field's menu offers %s; labels: %v", forbidden, labels(h))
		}
	}
	if len(changes) != 0 {
		t.Errorf("a ReadOnly field reported changes: %q", changes)
	}
}

// An edit menu closes when the widget that opened it leaves the tree.
//
// The menu is an overlay entry beside the tree, not a descendant of the
// field, so unmounting the field did not take it down: a route pop or a list
// scroll-out left the scrim and the Cut/Copy/Paste bar up, with actions bound
// to a state that was gone.
func TestEditMenuClosesWithItsOpener(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opener widget.Widget
	}{
		{"TextField", widget.TextField{Value: "hello world"}},
		{"SelectableText", widget.SelectableText{S: "hello world"}},
		{"SelectionArea", widget.SelectionArea{Child: widget.Text{Value: "hello world"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			show := true
			root, m := newMutable(func(widget.Ctx) widget.Widget {
				if !show {
					return widget.Text{Value: "gone"}
				}
				return widget.Sized{W: 300, Child: tc.opener}
			})
			h := headless(t, widget.OverlayHost{Child: root}, 320, 240)
			h.Render()
			at := geom.Pt{X: 20, Y: 8}
			h.TouchPress(at)
			h.Step(0.7)
			h.TouchRelease(at)
			h.Render()
			if !hasLabel(h, "Copy") {
				t.Fatalf("no edit menu after a long press; labels: %v", labels(h))
			}

			show = false
			m.Rebuild()
			h.Render()
			if hasLabel(h, "Copy") {
				t.Fatalf("the edit menu outlived its opener; labels: %v", labels(h))
			}
		})
	}
}
