//go:build !js && !linux && !darwin

package desktop

import "github.com/doug/gophics/shell"

// Gamepads satisfies shell.GamepadWindow. There is no desktop controller backend
// yet, so the capability is present (a game can poll it every frame) but always
// reports no controllers. Poll is poll-safe: it never nil-derefs and returns an
// empty slice.
func (w *window) Gamepads() shell.Gamepads { return desktopGamepads{} }

type desktopGamepads struct{}

func (desktopGamepads) Poll() []shell.Gamepad {
	// TODO(platform): SDL/XInput/evdev/GameController
	return nil
}
