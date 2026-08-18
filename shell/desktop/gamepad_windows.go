//go:build windows && !js

// Windows implementation of the shell gamepad capability (shell/gamepad.go)
// over XInput.
//
// XInput rather than raw HID or DirectInput: Windows already normalises any
// Xbox-compatible controller — which in practice is most of them — to one fixed
// layout, so buttonA is buttonA without a per-vendor mapping table. That table
// is the part that ages badly, and the same reasoning picked GameController on
// macOS.
//
// The cost is XInput's own limit: four controllers, and no PlayStation-specific
// niceties. Both are acceptable next to maintaining vendor quirks by hand.

package desktop

import (
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/doug/gophics/shell"
)

// xinputMaxControllers is XInput's fixed ceiling (XUSER_MAX_COUNT).
const xinputMaxControllers = 4

// errDeviceNotConnected is ERROR_DEVICE_NOT_CONNECTED: an empty slot, which is
// the normal case rather than a failure.
const errDeviceNotConnected = 1167

var (
	xinputOnce      sync.Once
	procXInputState *windows.LazyProc
)

// loadXInput resolves XInputGetState, preferring the modern DLL.
//
// The version dance is unavoidable: xinput1_4 ships with Windows 8 and later,
// xinput9_1_0 is the ancient in-box fallback, and xinput1_3 arrives only with
// the DirectX redistributable — which a machine may simply not have. Trying
// them in order is what every XInput consumer ends up doing.
func loadXInput() *windows.LazyProc {
	xinputOnce.Do(func() {
		for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
			dll := windows.NewLazySystemDLL(name)
			if dll.Load() != nil {
				continue
			}
			p := dll.NewProc("XInputGetState")
			if p.Find() == nil {
				procXInputState = p
				return
			}
		}
	})
	return procXInputState
}

// xinputGamepad mirrors XINPUT_GAMEPAD.
type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// xinputState mirrors XINPUT_STATE.
type xinputState struct {
	PacketNumber uint32
	Gamepad      xinputGamepad
}

// XINPUT_GAMEPAD_* button bits.
const (
	padDPadUp        = 0x0001
	padDPadDown      = 0x0002
	padDPadLeft      = 0x0004
	padDPadRight     = 0x0008
	padStart         = 0x0010
	padBack          = 0x0020
	padLeftThumb     = 0x0040
	padRightThumb    = 0x0080
	padLeftShoulder  = 0x0100
	padRightShoulder = 0x0200
	padA             = 0x1000
	padB             = 0x2000
	padX             = 0x4000
	padY             = 0x8000
)

// Gamepads satisfies shell.GamepadWindow for the Windows desktop shell.
func (w *window) Gamepads() shell.Gamepads { return windowsGamepads{} }

type windowsGamepads struct{}

// Poll snapshots every connected controller. An empty result is the normal case
// with nothing plugged in, and never an error.
func (windowsGamepads) Poll() []shell.Gamepad {
	proc := loadXInput()
	if proc == nil {
		return nil
	}
	var out []shell.Gamepad
	for i := range xinputMaxControllers {
		var st xinputState
		r, _, _ := proc.Call(uintptr(i), uintptr(unsafe.Pointer(&st)))
		if r == errDeviceNotConnected {
			continue
		}
		if r != 0 {
			continue
		}
		out = append(out, xinputToGamepad(i, st.Gamepad))
	}
	return out
}

// xinputToGamepad converts one XINPUT_GAMEPAD into the portable snapshot.
//
// Button order matches the macOS and web shells so a widget reading Buttons[0]
// does not have to know which platform it is on.
func xinputToGamepad(index int, g xinputGamepad) shell.Gamepad {
	btn := func(mask uint16) float32 {
		if g.Buttons&mask != 0 {
			return 1
		}
		return 0
	}
	return shell.Gamepad{
		ID:        "XInput controller " + strconv.Itoa(index+1),
		Connected: true,
		Buttons: []float32{
			btn(padA), btn(padB), btn(padX), btn(padY),
			btn(padLeftShoulder), btn(padRightShoulder),
			// Triggers are analog on XInput and are reported as such, matching
			// the browser Gamepad API, where buttons 6 and 7 carry a value.
			float32(g.LeftTrigger) / 255, float32(g.RightTrigger) / 255,
			btn(padBack), btn(padStart),
			btn(padLeftThumb), btn(padRightThumb),
			btn(padDPadUp), btn(padDPadDown), btn(padDPadLeft), btn(padDPadRight),
		},
		Axes: []float32{
			axis(g.ThumbLX), -axis(g.ThumbLY),
			axis(g.ThumbRX), -axis(g.ThumbRY),
		},
	}
}

// axis maps an XInput thumbstick value onto -1..1.
//
// The negative range is one larger than the positive (-32768..32767), so
// dividing by 32767 would let a full-left push report slightly beyond -1.
// Clamping is cheaper than explaining why a stick reads -1.00003.
func axis(v int16) float32 {
	f := float32(v) / 32767
	if f < -1 {
		return -1
	}
	return f
}
