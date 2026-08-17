//go:build windows

// Hooking the UIA provider onto the Win32 window.

package platform

// wmGetObject is the message Windows sends to ask a window for an
// accessibility object. It is sent for several object models at once,
// distinguished by lParam; only UiaRootObjectId concerns us.
const wmGetObject = 0x003D

// SetA11yTree implements A11yWindow.
//
// The provider is created on the first published tree rather than at window
// creation: most windows never publish one, and it costs a COM object and a map
// entry. Windows is not told about the change here — it asks via WM_GETOBJECT
// when a client attaches, and re-reads properties on demand.
func (w *win32Window) SetA11yTree(nodes []A11yNode, activate func(id int)) {
	uiaProviderFor(w.hwnd).SetTree(nodes, activate)
}

// AnnounceA11y implements A11yWindow.
//
// Not yet wired. UIA delivers announcements as a NotificationEvent raised
// against a provider, which needs UiaRaiseNotificationEvent and a live client;
// a silent no-op is better than a fabricated one while the tree itself reads
// correctly.
func (w *win32Window) AnnounceA11y(message string, assertive bool) {}

// Verify the window satisfies the optional accessibility interface. Windows
// does not implement A11yAvailable: unlike Linux, where AT-SPI may simply not
// be running, UI Automation is part of the OS and always present.
var _ A11yWindow = (*win32Window)(nil)
