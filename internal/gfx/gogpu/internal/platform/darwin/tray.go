//go:build darwin

package darwin

import "sync"

// Menu-bar status items (NSStatusItem) — the macOS system tray.
//
// A status item is owned by NSStatusBar, not by any window, which is the point:
// it outlives the window and is how an app stays reachable after its last
// window closes. That also means it must be released explicitly, or the icon
// survives the app that put it there until the process exits.

var (
	trayOnce sync.Once
	traySels struct {
		systemStatusBar   SEL
		statusItemWithLen SEL
		removeStatusItem  SEL
		button            SEL
		setTitle          SEL
		setToolTip        SEL
		setMenu           SEL
	}
)

func initTraySelectors() {
	trayOnce.Do(func() {
		traySels.systemStatusBar = RegisterSelector("systemStatusBar")
		traySels.statusItemWithLen = RegisterSelector("statusItemWithLength:")
		traySels.removeStatusItem = RegisterSelector("removeStatusItem:")
		traySels.button = RegisterSelector("button")
		traySels.setTitle = RegisterSelector("setTitle:")
		traySels.setToolTip = RegisterSelector("setToolTip:")
		traySels.setMenu = RegisterSelector("setMenu:")
	})
}

// NSVariableStatusItemLength: the item sizes itself to its content.
const variableStatusItemLength = -1.0

// StatusItem is a live menu-bar item.
type StatusItem struct {
	id ID
}

// NewStatusItem adds an item to the menu bar showing title, with tooltip on
// hover. Returns nil if AppKit is unavailable.
//
// Must be called on the main thread, like everything else that touches AppKit's
// menus.
func NewStatusItem(title, tooltip string) *StatusItem {
	if err := initRuntime(); err != nil {
		return nil
	}
	initTraySelectors()
	ensureAppDelegate()

	bar := ID(GetClass("NSStatusBar")).Send(traySels.systemStatusBar)
	if bar.IsNil() {
		return nil
	}
	item := bar.SendDouble(traySels.statusItemWithLen, variableStatusItemLength)
	if item.IsNil() {
		return nil
	}
	s := &StatusItem{id: item}
	s.SetTitle(title)
	s.SetTooltip(tooltip)
	return s
}

// SetTitle changes the text shown in the menu bar.
//
// Set on the item's button rather than the item itself: -[NSStatusItem
// setTitle:] has been deprecated since 10.10 and does nothing on a modern
// system, which looks exactly like the item failing to appear.
func (s *StatusItem) SetTitle(title string) {
	if s == nil || s.id.IsNil() {
		return
	}
	btn := s.id.Send(traySels.button)
	if btn.IsNil() {
		return
	}
	if ns := createNSString(title); ns != 0 {
		btn.SendPtr(traySels.setTitle, uintptr(ns))
	}
}

func (s *StatusItem) SetTooltip(tooltip string) {
	if s == nil || s.id.IsNil() || tooltip == "" {
		return
	}
	btn := s.id.Send(traySels.button)
	if btn.IsNil() {
		return
	}
	if ns := createNSString(tooltip); ns != 0 {
		btn.SendPtr(traySels.setToolTip, uintptr(ns))
	}
}

// SetMenu attaches the menu shown when the item is clicked. Pass 0 to detach.
func (s *StatusItem) SetMenu(menu ID) {
	if s == nil || s.id.IsNil() {
		return
	}
	s.id.SendPtr(traySels.setMenu, uintptr(menu))
}

// NewTrayMenu builds an empty NSMenu for a status item.
//
// Not NewMainMenu: that one installs itself as the application menu bar, which
// would replace the app's menus with the tray's.
func NewTrayMenu() ID {
	if err := initRuntime(); err != nil {
		return 0
	}
	alloc := ID(GetClass("NSMenu")).Send(RegisterSelector("alloc"))
	if alloc.IsNil() {
		return 0
	}
	title := createNSString("")
	if title == 0 {
		return 0
	}
	return alloc.SendPtr(RegisterSelector("initWithTitle:"), uintptr(title))
}

// Remove takes the item out of the menu bar. Without this the icon outlives
// the app's interest in it, up to process exit.
func (s *StatusItem) Remove() {
	if s == nil || s.id.IsNil() {
		return
	}
	initTraySelectors()
	bar := ID(GetClass("NSStatusBar")).Send(traySels.systemStatusBar)
	if !bar.IsNil() {
		bar.SendPtr(traySels.removeStatusItem, s.id.Ptr())
	}
	s.id = 0
}
