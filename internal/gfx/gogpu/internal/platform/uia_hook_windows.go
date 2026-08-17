//go:build windows

// Hooking the UIA provider onto the Win32 window.

package platform

import (
	"fmt"
	"os"
	"sync"
)

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
	uiaLogf("SetA11yTree nodes=%d hwnd=%#x", len(nodes), uintptr(w.hwnd))
	uiaProviderFor(w.hwnd).SetTree(nodes, activate)
}

// GOPHICS_UIA_DEBUG turns on tracing for the provider handshake, which is
// otherwise invisible: a client that gets no tree and a window that is never
// asked look identical from the outside.
//
// Setting it to a path writes there instead of to stderr. That matters more
// than it sounds: a GUI app launched into the interactive desktop session — the
// only session a screen reader can see — has no console to inherit, so stderr
// goes nowhere and shell redirection is not available to the launcher.
var uiaDebug = os.Getenv("GOPHICS_UIA_DEBUG")

var uiaLogMu sync.Mutex

func uiaLogf(format string, args ...any) {
	if uiaDebug == "" {
		return
	}
	line := "uia: " + fmt.Sprintf(format, args...) + "\n"
	if uiaDebug == "1" || uiaDebug == "stderr" {
		fmt.Fprint(os.Stderr, line)
		return
	}
	uiaLogMu.Lock()
	defer uiaLogMu.Unlock()
	f, err := os.OpenFile(uiaDebug, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
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
