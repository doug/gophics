//go:build !js

// Desktop implementation of the accessibility capability (shell/a11y.go).
//
// gophics draws every pixel, so the OS sees one opaque view: nothing in a
// gophics window is reachable by a screen reader unless the app publishes what
// it drew. This forwards the app's semantics to the windowing layer, which
// owns the per-OS bridge (AppKit's NSAccessibility today; see
// internal/gfx/gogpu/internal/platform/darwin/a11y.go).
//
// The capability is only published when the running platform actually has a
// bridge. That is deliberate: ctx.Accessibility() returning nil is how a
// caller learns the answer, and a capability that silently discards everything
// is worse than an absent one, because an app cannot tell the difference.
package desktop

import (
	"github.com/doug/gophics/internal/gfx/gogpu"
	"github.com/doug/gophics/shell"
)

// Accessibility publishes the capability; the app runner wires it to the
// widget tree. It returns nil where the platform has no bridge.
func (w *window) Accessibility() shell.Accessibility {
	if !a11ySupported(w.app) {
		return nil
	}
	return desktopA11y{w: w}
}

// a11ySupported probes the windowing layer without publishing anything: an
// empty tree is a no-op on a platform that has a bridge and reports false on
// one that does not.
func a11ySupported(app *gogpu.App) bool {
	if app == nil {
		return false
	}
	return app.SetAccessibilityTree(nil, nil)
}

type desktopA11y struct{ w *window }

// SetTree converts the shell's nodes to the windowing layer's and publishes
// them. The two types are field-identical by design — the windowing layer must
// not depend on gophics — so this is a copy, not a translation.
func (a desktopA11y) SetTree(nodes []shell.A11yNode, activate func(id int)) {
	out := make([]gogpu.A11yNode, len(nodes))
	for i, n := range nodes {
		out[i] = gogpu.A11yNode{
			ID: n.ID, ParentID: n.ParentID,
			Role: n.Role, Label: n.Label, Value: n.Value, Hint: n.Hint,
			X: n.X, Y: n.Y, W: n.W, H: n.H,
			Tappable: n.Tappable, Focused: n.Focused, Disabled: n.Disabled,
			Selected: n.Selected, Checkable: n.Checkable, Checked: n.Checked,
			// TODO(platform): Expandable/Expanded are dropped here. Carrying
			// them needs the field on the substrate's A11yNode and a mapping in
			// each backend — AT-SPI EXPANDABLE/EXPANDED states, UIA's
			// ExpandCollapsePattern, NSAccessibilityDisclosureTriangle. Web
			// already emits aria-expanded, so a tree announces its state there
			// and not yet on the desktop.
		}
	}
	// activate is passed straight through. It arrives on whichever thread the
	// platform calls us from, but the marshalling is already handled one layer
	// up: Accessibility gained a callback parameter when SetTree was added, so
	// capgen now wraps it in PostedAccessibility, which delivers on the UI
	// goroutine.
	//
	// This used to wrap it in runOnMain as well. That was the wrong hop even
	// before the wrapper existed — runOnMain marshals to the AppKit *main
	// thread*, which matters for NSOpenPanel and not at all for widget state —
	// and with the wrapper in place it queued the callback twice.
	a.w.app.SetAccessibilityTree(out, activate)
}

func (a desktopA11y) Announce(message string, assertive bool) {
	a.w.app.AnnounceAccessibility(message, assertive)
}
