package shell

// Native application menus. A Window opts in by implementing MenuWindow;
// widgets reach it via ctx.Menus(), nil where unsupported. Callbacks fire on
// the UI goroutine.
//
// Menus are the one desktop affordance an app cannot draw for itself: on macOS
// the menu bar lives outside the window entirely, and on every platform users
// expect the OS's own keyboard handling, accessibility and conventions. So this
// publishes a description and lets the platform build the real thing.
//
// Items carry an ID rather than a closure, and activation arrives through a
// callback passed to SetBar — the same shape as Accessibility.SetTree, and for
// the same two reasons. A closure cannot cross into a platform menu that
// outlives the build that created it, and a callback parameter is what lets
// capgen wrap the capability so activations land on the UI goroutine instead of
// whichever thread the OS used.

// MenuWindow is implemented by a Window that can publish a native menu bar.
type MenuWindow interface {
	Menus() Menus
}

// Menus publishes the application menu bar.
type Menus interface {
	// SetBar replaces the menu bar with bar. invoke is called with the ID of
	// the item the user chose.
	//
	// Publishing an empty slice removes the app's menus, leaving whatever the
	// platform provides by default.
	SetBar(bar []Menu, invoke func(id int))
}

// Menu is one top-level menu in the bar.
type Menu struct {
	Title string
	Items []MenuItem
}

// MenuItem is one row. A Separator ignores every other field; an item with a
// Submenu ignores ID and Role.
type MenuItem struct {
	// ID is the app's own identifier, handed back to invoke. Zero is valid;
	// items with no action can leave it unset and simply never fire.
	ID        int
	Title     string
	Role      MenuRole
	Disabled  bool
	Separator bool
	Submenu   []MenuItem
}

// MenuRole marks an item the operating system places and names itself.
//
// This matters most on macOS, where About, Preferences, Services, Hide and Quit
// belong to the application menu and are expected in a particular order with
// particular shortcuts. An app that hand-rolls them gets a menu bar that looks
// almost right, which is worse than one that looks plainly custom. Platforms
// without the convention treat a role as an ordinary item and use Title.
type MenuRole int

const (
	RoleNone MenuRole = iota
	RoleAbout
	RolePreferences
	RoleServices
	RoleHide
	RoleHideOthers
	RoleShowAll
	RoleQuit
	RoleClose
	RoleMinimize
	RoleZoom
	RoleFullScreen
	RoleBringAllToFront
)
