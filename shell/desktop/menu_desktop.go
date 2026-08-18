//go:build !js

// Desktop implementation of the menu capability (shell/menu.go).
//
// The platform work already exists: gogpu builds real NSMenu, HMENU and GTK-less
// X11 menus from its own Menu model, with roles placed by the OS. This is the
// translation, and it is deliberately a copy rather than a re-export — the shell
// API must not leak the windowing layer's types, the same rule a11y_desktop.go
// follows for nodes.

package desktop

import (
	"github.com/doug/gophics/internal/gfx/gogpu"
	"github.com/doug/gophics/shell"
)

// Menus publishes the capability; the app runner wires it to the widget tree.
// It returns nil where the platform has no menu bar to publish into.
func (w *window) Menus() shell.Menus {
	if w.app == nil {
		return nil
	}
	return desktopMenus{w: w}
}

type desktopMenus struct{ w *window }

// SetBar converts the description and hands it to gogpu.
//
// Each item's Action closes over its ID and the invoke callback rather than over
// app state. That is what lets a platform menu outlive the build that created
// it: the menu bar persists until replaced, while the widget tree that described
// it is rebuilt every frame.
// The whole call runs on the platform's main thread. AppKit is emphatic about
// this — "Main menu contents may only be modified from the main thread", raised
// as an uncatchable NSInternalInconsistencyException — and Build runs on the UI
// goroutine, which is not it.
//
// Worth contrasting with a11y_desktop.go, which had runOnMain removed. The two
// look similar and want opposite things: an accessibility activation must reach
// widget state, so it belongs on the UI goroutine, while a menu belongs to
// AppKit and must be built where AppKit lives. Marshalling is not one hop that
// fits everywhere.
func (m desktopMenus) SetBar(bar []shell.Menu, invoke func(id int)) {
	m.w.runOnMain(func() { m.setBar(bar, invoke) })
}

func (m desktopMenus) setBar(bar []shell.Menu, invoke func(id int)) {
	if len(bar) == 0 {
		m.w.app.SetMenu(nil)
		return
	}
	// gogpu's SetMenu takes one root; the bar's entries become its submenus,
	// which is how a menu bar is modelled on every platform underneath.
	root := gogpu.NewMenu()
	for _, menu := range bar {
		root.AddItem(gogpu.MenuItem{
			Title:   menu.Title,
			Submenu: convertMenu(menu.Title, menu.Items, invoke),
		})
	}
	m.w.app.SetMenu(root)
}

// convertMenu builds a gogpu menu from shell items.
func convertMenu(title string, items []shell.MenuItem, invoke func(id int)) *gogpu.Menu {
	out := gogpu.NewMenuWithTitle(title)
	for _, it := range items {
		out.AddItem(convertItem(it, invoke))
	}
	return out
}

func convertItem(it shell.MenuItem, invoke func(id int)) gogpu.MenuItem {
	if it.Separator {
		return gogpu.NewSeparator()
	}
	out := gogpu.MenuItem{
		Title:    it.Title,
		Role:     gogpu.MenuRole(it.Role),
		Disabled: it.Disabled,
	}
	if len(it.Submenu) > 0 {
		out.Submenu = convertMenu(it.Title, it.Submenu, invoke)
		return out
	}
	// A role is handled by the OS — Quit really quits — so attaching an action
	// as well would run the app's handler *and* the platform's.
	if it.Role == shell.RoleNone && invoke != nil {
		id := it.ID
		out.Action = func() { invoke(id) }
	}
	return out
}
