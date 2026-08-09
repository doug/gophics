package mobile

import "github.com/doug/gophics/shell"

// Gamepads makes the Bridge a shell.GamepadWindow. There is no mobile controller
// backend yet, so the capability is present (a game can poll it every frame) but
// always reports no controllers. Poll is poll-safe: it never nil-derefs and
// returns an empty slice.
func (b *Bridge) Gamepads() shell.Gamepads { return bridgeGamepads{} }

type bridgeGamepads struct{}

func (bridgeGamepads) Poll() []shell.Gamepad {
	// TODO(platform): iOS GameController / Android InputDevice, drained over the
	// Bridge host like the media/haptic capabilities.
	return nil
}
