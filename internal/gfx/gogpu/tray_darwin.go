//go:build darwin && !ios

package gogpu

import (
	"github.com/doug/gophics/internal/gfx/gogpu/internal/platform/darwin"
)

// trayItem is the live menu-bar item, or nil when nothing is shown.
var trayItem *darwin.StatusItem

// ShowTray adds or updates the application's menu-bar item.
//
// It belongs to the application rather than a window: a status item outlives
// the window that asked for it, which is the whole reason to have one.
//
// Reports whether the platform did it, so a caller can tell "no tray here" from
// "shown", rather than assuming.
//
// Must be called on the platform's main thread, like SetMenu: AppKit refuses
// menu mutation anywhere else, and raises an exception Go cannot catch. The
// shell binding wraps it in runOnMain.
func (a *App) ShowTray(title, tooltip string, items []MenuItem) bool {
	if trayItem == nil {
		trayItem = darwin.NewStatusItem(title, tooltip)
		if trayItem == nil {
			return false
		}
	} else {
		trayItem.SetTitle(title)
		trayItem.SetTooltip(tooltip)
	}
	menu := darwin.NewTrayMenu()
	if menu != 0 {
		addTrayItems(menu, items)
		trayItem.SetMenu(menu)
	}
	return true
}

// addTrayItems fills an NSMenu from the portable item list.
func addTrayItems(menu darwin.ID, items []MenuItem) {
	for _, it := range items {
		if it.Separator {
			darwin.AddSeparatorItem(menu)
			continue
		}
		darwin.AddMenuItemWithCallback(menu, it.Title, it.Action, "")
	}
}

// HideTray removes the menu-bar item. Main thread, as above.
func (a *App) HideTray() {
	if trayItem != nil {
		trayItem.Remove()
		trayItem = nil
	}
}
