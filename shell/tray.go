package shell

// System tray / menu-bar items. A Window opts in by implementing TrayWindow;
// widgets reach it via ctx.Tray(), nil where unsupported. Callbacks fire on the
// UI goroutine.
//
// The tray is what lets a desktop app outlive its window — a sync client, a
// clipboard tool, anything that should keep running when the last window
// closes. It belongs to the application rather than to any window, which is why
// the item persists until Hide and does not follow the window that created it.
//
// The menu reuses the type from shell/menu.go, because a tray menu is a menu.
// IDs and the invoke callback work the same way and for the same reasons: the
// item outlives the build that described it, so it cannot hold closures.

// TrayWindow is implemented by a Window that can publish a tray item.
type TrayWindow interface {
	Tray() Tray
}

// Tray publishes one item in the system tray or menu bar.
type Tray interface {
	// Show adds or updates the item. invoke is called with the ID of the menu
	// entry chosen. Calling it again replaces what is shown.
	Show(item TrayItem, invoke func(id int))
	// Hide removes the item. An app that exits without calling this can leave
	// its icon behind until the process is reaped.
	Hide()
}

// TrayItem describes what to show.
type TrayItem struct {
	// Title is the text shown in the bar. On macOS this is the usual way to
	// label a menu-bar item; elsewhere it is the fallback when no icon is set.
	Title string
	// Tooltip is shown on hover.
	Tooltip string
	// Menu is shown when the item is clicked. An item with no menu is display
	// only, which is rarely what a user expects — there is no other way to
	// interact with a tray icon.
	Menu []MenuItem
}
