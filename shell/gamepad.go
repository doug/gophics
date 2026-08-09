package shell

// Gamepad input. A Window exposes it by implementing GamepadWindow; callers
// reach it through the widget layer (ctx.Gamepads()), which returns nil when the
// running platform can't provide one. Only the web shell implements it today.
//
// Unlike the media/file capabilities, gamepad input is poll-style, matching the
// input package (input.State): there are no callbacks. Read the current state
// once per frame with Poll() and diff it yourself if you need edge events. This
// mirrors how the browser Gamepad API works — the spec deliberately snapshots
// controller state per frame rather than dispatching per-button events.

// GamepadWindow is implemented by a Window that can read game controllers. The
// app runner type-asserts the Window to it and, when present, publishes
// Gamepads() to the widget tree — the same shape as MediaWindow/FilePickerWindow.
type GamepadWindow interface {
	// Gamepads returns the poll-style controller capability, or nil if
	// unavailable.
	Gamepads() Gamepads
}

// Gamepads reads connected game controllers. It is poll-style: call Poll() each
// frame. It reports no controllers (an empty slice) rather than erroring when
// none are attached, so a game can call it unconditionally every frame.
type Gamepads interface {
	// Poll returns a snapshot of the connected controllers this frame. The
	// result is empty when nothing is plugged in — that is the normal, expected
	// case and is never an error.
	Poll() []Gamepad
}

// Gamepad is a single controller's state for one frame.
type Gamepad struct {
	// ID identifies the controller (make/model string, platform-defined).
	ID string
	// Buttons holds each button's analog value in 0..1 (1 = fully pressed;
	// digital buttons report 0 or 1). Layout is platform/controller-defined.
	Buttons []float32
	// Axes holds each analog axis in -1..1 (sticks, triggers-as-axes).
	Axes []float32
	// Connected reports whether this controller is currently attached.
	Connected bool
}
