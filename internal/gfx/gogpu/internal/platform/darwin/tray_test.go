//go:build darwin

package darwin

import "testing"

// Creating a real NSStatusItem cannot be tested here, and the attempt is worse
// than useless: -[NSStatusItem init] builds an NSStatusBarWindow, AppKit
// refuses NSWindow off the main thread, and the exception is uncatchable, so it
// terminates the whole test binary rather than failing one case. A Go test has
// no AppKit main loop to run on.
//
// So the tests below cover what can be checked off the main thread — that the
// selectors resolve and that teardown is safe — and creation is verified by
// running examples/menus, where the shell binding puts the call on the main
// thread via runOnMain.

// Remove must be safe more than once and on a zero value: an app that hides its
// tray on shutdown should not have to track whether it already did.
func TestStatusItemRemoveIsIdempotent(t *testing.T) {
	var s *StatusItem
	s.Remove() // nil receiver

	empty := &StatusItem{}
	empty.Remove()
	empty.Remove()
}

// The selector table must resolve; a typo yields a nil SEL and a message that
// goes nowhere, which looks like the tray silently not working.
func TestTraySelectorsResolve(t *testing.T) {
	if err := initRuntime(); err != nil {
		t.Skip(err)
	}
	initTraySelectors()
	for name, sel := range map[string]SEL{
		"systemStatusBar":      traySels.systemStatusBar,
		"statusItemWithLength": traySels.statusItemWithLen,
		"removeStatusItem":     traySels.removeStatusItem,
		"button":               traySels.button,
		"setTitle":             traySels.setTitle,
		"setToolTip":           traySels.setToolTip,
		"setMenu":              traySels.setMenu,
	} {
		if sel == 0 {
			t.Errorf("selector %s did not resolve", name)
		}
	}
}
