//go:build !(darwin && !ios)

package gogpu

// ShowTray reports false where no tray backend exists yet.
//
// Windows has Shell_NotifyIcon and Linux has StatusNotifierItem over D-Bus —
// both reachable from pure Go, and the D-Bus half of the Linux one already
// exists for AT-SPI. Neither is written; saying so lets a caller hide the
// affordance rather than showing one that does nothing.
func (a *App) ShowTray(title, tooltip string, items []MenuItem) bool { return false }

// HideTray is a no-op where ShowTray never showed anything.
func (a *App) HideTray() {}
