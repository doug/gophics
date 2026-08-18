//go:build !js

// Desktop implementation of the tray capability (shell/tray.go).

package desktop

import (
	"github.com/doug/gophics/internal/gfx/gogpu"
	"github.com/doug/gophics/shell"
)

// Tray publishes the capability, or nil where the platform has no tray
// backend — today that is everywhere but macOS.
//
// Probing by showing nothing would put an empty item in the user's menu bar, so
// support is asked for by build rather than by trial.
func (w *window) Tray() shell.Tray {
	if w.app == nil || !gogpu.TraySupported() {
		return nil
	}
	return desktopTray{w: w}
}

type desktopTray struct{ w *window }

// Show installs or updates the item.
//
// On the main thread, like menus: AppKit refuses menu mutation anywhere else,
// and the exception it raises cannot be caught from Go. See menu_desktop.go for
// why this hop is right here and wrong for accessibility activation.
func (t desktopTray) Show(item shell.TrayItem, invoke func(id int)) {
	items := make([]gogpu.MenuItem, 0, len(item.Menu))
	for _, mi := range item.Menu {
		items = append(items, convertItem(mi, invoke))
	}
	t.w.runOnMain(func() { t.w.app.ShowTray(item.Title, item.Tooltip, items) })
}

func (t desktopTray) Hide() {
	t.w.runOnMain(func() { t.w.app.HideTray() })
}
