//go:build darwin && !ios && !js

// macOS implementation of the shell gamepad capability (shell/gamepad.go) over
// GameController.framework, driven through the pure-Go Objective-C bridge.
//
// GameController rather than IOHIDManager: macOS already does the work of
// recognising a DualSense or an Xbox controller and presenting it as a known
// layout, so buttonA is buttonA on every pad. Going to HID directly would mean
// re-implementing that mapping per vendor, which is the part that ages badly.
//
// Controllers that are paired and awake appear in +[GCController controllers]
// without starting discovery. Wireless discovery is a separate, asynchronous
// call that pops system UI, which a poll-style capability has no business
// triggering on the caller's behalf.

package desktop

import (
	"sync"

	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

var gcOnce sync.Once

// Gamepads satisfies shell.GamepadWindow for the macOS desktop shell.
func (w *window) Gamepads() shell.Gamepads { return darwinGamepads{} }

type darwinGamepads struct{}

// buttonSelectors is the reported button order. It is fixed so Buttons[3] means
// the same thing every poll and across controllers, and it follows the order
// the web shell's callers already see from the browser Gamepad API.
var buttonSelectors = []string{
	"buttonA", "buttonB", "buttonX", "buttonY",
	"leftShoulder", "rightShoulder", "leftTrigger", "rightTrigger",
	"buttonOptions", "buttonMenu",
	"leftThumbstickButton", "rightThumbstickButton",
}

// dpadSelectors follow the buttons, matching the standard layout's 12..15.
var dpadSelectors = []string{"up", "down", "left", "right"}

// Poll snapshots every connected controller. It returns an empty slice when
// nothing is attached, which is the normal case and never an error.
func (darwinGamepads) Poll() []shell.Gamepad {
	gcOnce.Do(func() { _ = objc.LoadFramework("GameController") })

	cls := objc.Class("GCController")
	if !cls.Valid() {
		return nil
	}
	list := cls.Send("controllers")
	if !list.Valid() {
		return nil
	}

	var out []shell.Gamepad
	for _, c := range objc.Array(list) {
		// extendedGamepad is nil for a remote or a plain micro-gamepad; those
		// have no sticks and do not fit the reported layout.
		pad := c.Send("extendedGamepad")
		if !pad.Valid() {
			continue
		}
		out = append(out, readExtendedGamepad(c, pad))
	}
	return out
}

// readExtendedGamepad reads one GCExtendedGamepad into a frame snapshot.
func readExtendedGamepad(controller, pad objc.ID) shell.Gamepad {
	g := shell.Gamepad{ID: controllerName(controller), Connected: true}

	for _, sel := range buttonSelectors {
		// A button absent on this model messages nil, which the bridge answers
		// with zero — an unpressed button, which is what it is.
		g.Buttons = append(g.Buttons, pad.Send(sel).SendFloat("value"))
	}
	dpad := pad.Send("dpad")
	for _, sel := range dpadSelectors {
		g.Buttons = append(g.Buttons, dpad.Send(sel).SendFloat("value"))
	}

	for _, stick := range []string{"leftThumbstick", "rightThumbstick"} {
		s := pad.Send(stick)
		x := s.Send("xAxis").SendFloat("value")
		// GameController points y up; the Gamepad API that the web shell
		// reports points it down. Negate here so a widget reading Axes[1] does
		// not have to know which platform it is on.
		y := -s.Send("yAxis").SendFloat("value")
		g.Axes = append(g.Axes, x, y)
	}
	return g
}

// controllerName prefers the vendor's name, falling back to a generic label so
// the ID is never empty.
func controllerName(c objc.ID) string {
	if n := objc.GoString(c.Send("vendorName")); n != "" {
		return n
	}
	return "gamepad"
}
