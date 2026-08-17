//go:build linux

// Hooking the AT-SPI bridge onto the Linux windows.
//
// One host serves both X11 and Wayland. AT-SPI is a D-Bus protocol and knows
// nothing about the display server, so unlike almost everything else in this
// package there is no reason to write it twice.
//
// The bridge is created lazily, on the first accessibility call, and the result
// is cached — including the failure. Connecting means a socket, a SASL exchange
// and a Hello round-trip, which is not work to do at startup for the many users
// who have no screen reader running.

package platform

import (
	"os"
	"path/filepath"
	"sync"
)

// atspiHost is embedded in the platform window types to give them an
// accessibility bridge.
type atspiHost struct {
	once   sync.Once
	bridge *atspiBridge
}

// get returns the bridge, creating it on first use. nil means this machine has
// no accessibility bus — the ordinary case on a desktop with no assistive
// technology enabled, and not an error.
func (h *atspiHost) get() *atspiBridge {
	h.once.Do(func() { h.bridge = newATSPIBridge(a11yAppName()) })
	return h.bridge
}

// A11yAvailable reports whether AT-SPI is reachable, so a machine without it
// leaves ctx.Accessibility() nil rather than handing back a capability that
// silently discards every tree it is given.
func (h *atspiHost) A11yAvailable() bool { return h.get() != nil }

// SetA11yTree implements A11yWindow.
func (h *atspiHost) SetA11yTree(nodes []A11yNode, activate func(id int)) {
	if b := h.get(); b != nil {
		b.SetTree(nodes, activate)
	}
}

// AnnounceA11y implements A11yWindow.
//
// Announcements are a documented no-op for now. AT-SPI delivers them as
// object:announcement events, which means emitting signals — the piece the
// bridge does not have yet — and a silent no-op is better than a fabricated
// one, since the tree itself is already being read correctly.
func (h *atspiHost) AnnounceA11y(message string, assertive bool) {}

// a11yAppName is what the screen reader announces when focus enters the
// window. The executable's name is not a great title, but it is honest and it
// is available here; the window title lives on the display-server side of this
// package and differs between X11 and Wayland.
func a11yAppName() string {
	if len(os.Args) > 0 && os.Args[0] != "" {
		if base := filepath.Base(os.Args[0]); base != "." && base != "/" {
			return base
		}
	}
	return "gophics"
}
