//go:build !js && !linux && !darwin && !windows

package desktop

import "github.com/doug/gophics/shell"

// Fallback for platforms with no controller backend — the BSDs, in practice.
// macOS uses GameController, Linux evdev and Windows XInput; see the
// per-platform files beside this one.
//
// The capability is still published so a game can poll it every frame without
// a nil check; it simply reports no controllers.
func (w *window) Gamepads() shell.Gamepads { return desktopGamepads{} }

type desktopGamepads struct{}

func (desktopGamepads) Poll() []shell.Gamepad {
	// TODO(platform): the BSDs — FreeBSD/OpenBSD expose controllers through
	// usbhid and uhid, which is a different model again from evdev.
	return nil
}
