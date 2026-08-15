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
		}
	}
	// Activation arrives on the platform's UI thread. gophics widget state is
	// owned by the UI goroutine, so the callback is queued rather than run
	// where the OS happened to call us.
	var onActivate func(int)
	if activate != nil {
		onActivate = func(id int) {
			a.w.runOnMain(func() { activate(id) })
		}
	}
	a.w.app.SetAccessibilityTree(out, onActivate)
}

func (a desktopA11y) Announce(message string, assertive bool) {
	a.w.app.AnnounceAccessibility(message, assertive)
}
